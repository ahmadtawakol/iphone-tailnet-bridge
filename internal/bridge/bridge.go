package bridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/bonjour"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
	localnetwork "github.com/ahmadtawakol/iphone-tailnet-bridge/internal/network"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/portscan"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/relay"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/tailscale"
)

type State string

const (
	StateOff         State = "off"
	StateStarting    State = "starting"
	StateActive      State = "active"
	StateDegraded    State = "degraded"
	StateWaiting     State = "waiting"
	StateLocal       State = "local"
	StateUnreachable State = "unreachable"
	StateNeedsWiFi   State = "needs_wifi"
	StateError       State = "error"
)

type Status struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	State     State          `json:"state"`
	Message   string         `json:"message"`
	Interface string         `json:"interface,omitempty"`
	LocalIP   string         `json:"localIP,omitempty"`
	TargetIP  string         `json:"targetIP,omitempty"`
	StartedAt *time.Time     `json:"startedAt,omitempty"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Relay     relay.Snapshot `json:"relay"`
}

func (state State) DisplayName() string {
	switch state {
	case StateNeedsWiFi:
		return "Wi-Fi required"
	case StateOff:
		return "Off"
	case StateStarting:
		return "Starting"
	case StateActive:
		return "Active"
	case StateDegraded:
		return "Degraded"
	case StateWaiting:
		return "Waiting"
	case StateLocal:
		return "Local"
	case StateUnreachable:
		return "Unreachable"
	case StateError:
		return "Error"
	default:
		return string(state)
	}
}

type UpdateFunc func(Status)

type reportedError struct {
	err error
}

func (reported reportedError) Error() string { return reported.err.Error() }
func (reported reportedError) Unwrap() error { return reported.err }

type Controller struct {
	Profile   config.Profile
	Tailscale *tailscale.Client
	Update    UpdateFunc
}

func (controller *Controller) Run(ctx context.Context) error {
	controller.report(Status{State: StateStarting, Message: "Preparing bridge"})
	for {
		if ctx.Err() != nil {
			controller.report(Status{State: StateOff, Message: "Bridge stopped"})
			return nil
		}
		err := controller.runGeneration(ctx)
		if ctx.Err() != nil {
			controller.report(Status{State: StateOff, Message: "Bridge stopped"})
			return nil
		}
		var alreadyReported reportedError
		if err != nil && !errors.As(err, &alreadyReported) {
			controller.report(Status{State: StateError, Message: err.Error()})
		}
		select {
		case <-ctx.Done():
			controller.report(Status{State: StateOff, Message: "Bridge stopped"})
			return nil
		case <-time.After(5 * time.Second):
		}
	}
}

func (controller *Controller) runGeneration(ctx context.Context) error {
	local, err := localnetwork.SelectLocalInterface(controller.Profile.Interface)
	if err != nil {
		controller.report(Status{State: StateWaiting, Message: err.Error()})
		return reportedError{err: err}
	}
	targetIP, peerOnline, err := controller.resolveTarget(ctx)
	if err != nil {
		controller.report(Status{State: StateWaiting, Message: err.Error(), Interface: local.Interface.Name, LocalIP: local.IPv4.String()})
		return reportedError{err: err}
	}
	base := Status{Interface: local.Interface.Name, LocalIP: local.IPv4.String(), TargetIP: targetIP.String()}
	present, detectErr := controller.localDevicePresent(ctx, local.Interface, local.IPv4, targetIP, 6*time.Second)
	if detectErr == nil && present {
		base.State = StateLocal
		base.Message = "The iPhone is already on this LAN; using Apple's normal Bonjour connection"
		controller.report(base)
		return reportedError{err: errors.New(base.Message)}
	}
	if !peerOnline {
		base.State = StateUnreachable
		base.Message = "The iPhone is offline in Tailscale"
		controller.report(base)
		return reportedError{err: errors.New(base.Message)}
	}
	if !reachable(targetIP, controller.Profile.PairingPort(), 3*time.Second) {
		base.State = StateNeedsWiFi
		base.Message = "Wake and unlock the iPhone, then connect it to any Wi-Fi. Apple RemotePairing is unavailable on cellular-only connections."
		controller.report(base)
		return reportedError{err: errors.New(base.Message)}
	}
	runtimeProfile, adjustedPorts := controller.refreshServicePorts(ctx, targetIP)
	engine, err := relay.New(local.IPv4, targetIP, runtimeProfile.AllowLANClients)
	if err != nil {
		return err
	}
	if err := engine.Start(runtimeProfile); err != nil {
		return err
	}
	defer engine.Stop()
	publisher := &bonjour.Publisher{}
	if err := publisher.Start(runtimeProfile.Services, local.Interface, local.IPv4); err != nil {
		return err
	}
	defer publisher.Stop()
	startedAt := time.Now().UTC()
	relayStatus := engine.Snapshot()
	base.StartedAt = &startedAt
	base.Relay = relayStatus
	if len(relayStatus.Issues) > 0 {
		base.State = StateDegraded
		base.Message = fmt.Sprintf("Bridge active with %d unavailable optional listeners", len(relayStatus.Issues))
	} else {
		base.State = StateActive
		base.Message = "Bridge active; Xcode can discover the iPhone"
	}
	if adjustedPorts > 0 {
		base.Message += fmt.Sprintf("; refreshed %d dynamic service port(s)", adjustedPorts)
	}
	controller.report(base)

	healthTicker := time.NewTicker(5 * time.Second)
	defer healthTicker.Stop()
	localTicker := time.NewTicker(20 * time.Second)
	defer localTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-healthTicker.C:
			if !publisher.Healthy() {
				return fmt.Errorf("Bonjour publisher stopped; rebuilding the bridge")
			}
			if !reachable(targetIP, controller.Profile.PairingPort(), 2*time.Second) {
				return fmt.Errorf("the iPhone RemotePairing listener became unreachable; rebuilding the bridge")
			}
			currentLocal, currentErr := localnetwork.SelectLocalInterface(controller.Profile.Interface)
			if currentErr != nil || currentLocal.Interface.Name != local.Interface.Name || !currentLocal.IPv4.Equal(local.IPv4) {
				return fmt.Errorf("the Mac network changed; rebuilding the bridge")
			}
			currentTarget, online, resolveErr := controller.resolveTarget(ctx)
			if resolveErr != nil || !online || !currentTarget.Equal(targetIP) {
				return fmt.Errorf("the iPhone Tailscale path changed; rebuilding the bridge")
			}
			base.Relay = engine.Snapshot()
			if len(base.Relay.Issues) > 0 {
				base.State = StateDegraded
				base.Message = fmt.Sprintf("Bridge active with %d unavailable optional listeners", len(base.Relay.Issues))
			} else {
				base.State = StateActive
				base.Message = "Bridge active; Xcode can discover the iPhone"
			}
			controller.report(base)
		case <-localTicker.C:
			present, _ := controller.localDevicePresent(ctx, local.Interface, local.IPv4, targetIP, 6*time.Second)
			if present {
				return fmt.Errorf("the iPhone returned to this LAN; withdrawing the proxy")
			}
		}
	}
}

func (controller *Controller) refreshServicePorts(ctx context.Context, targetIP net.IP) (config.Profile, int) {
	profile := controller.Profile
	adjusted := 0
	for index, service := range profile.Services {
		if service.Type == "_remotepairing._tcp" {
			continue
		}
		probeContext, cancel := context.WithTimeout(ctx, time.Second)
		reachable := portscan.ReachableTCP(probeContext, targetIP, service.Port, 750*time.Millisecond)
		cancel()
		if reachable {
			continue
		}
		first, last := 30000, 49999
		if service.Type == "_remoted._tcp" {
			first, last = 50000, 65535
		}
		scanContext, cancelScan := context.WithTimeout(ctx, 15*time.Second)
		port, found := portscan.FirstOpenTCP(scanContext, targetIP, portscan.Options{
			First:       first,
			Last:        last,
			Timeout:     300 * time.Millisecond,
			Concurrency: 512,
		})
		cancelScan()
		if found && port != profile.PairingPort() && !servicePortInUse(profile.Services, port, index) {
			profile.Services[index].Port = port
			adjusted++
		}
	}
	return profile, adjusted
}

func servicePortInUse(services []config.BonjourService, port, except int) bool {
	for index, service := range services {
		if index != except && service.Port == port {
			return true
		}
	}
	return false
}

func (controller *Controller) resolveTarget(ctx context.Context) (net.IP, bool, error) {
	if controller.Profile.Provider == config.ProviderManual {
		return controller.cachedTarget()
	}
	if controller.Tailscale == nil {
		return controller.cachedTarget()
	}
	peer, err := controller.Tailscale.Resolve(ctx, controller.Profile.Peer.NodeID, controller.Profile.Peer.Name)
	if err != nil {
		return controller.cachedTarget()
	}
	ip := net.ParseIP(peer.IPv4())
	if ip == nil || ip.To4() == nil {
		return nil, false, fmt.Errorf("Tailscale peer %s has no IPv4 address", peer.DisplayName())
	}
	return ip.To4(), peer.Online, nil
}

func (controller *Controller) cachedTarget() (net.IP, bool, error) {
	ip := net.ParseIP(controller.Profile.Peer.Address)
	if ip == nil || ip.To4() == nil {
		return nil, false, fmt.Errorf("the saved iPhone tailnet address is invalid")
	}
	return ip.To4(), true, nil
}

func (controller *Controller) localDevicePresent(parent context.Context, iface net.Interface, localIP, targetIP net.IP, duration time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, duration)
	defer cancel()
	candidates, err := bonjour.Discover(ctx, &iface)
	if err != nil {
		return false, err
	}
	profileHost := ""
	if len(controller.Profile.Services) > 0 {
		profileHost = controller.Profile.Services[0].Hostname
	}
	for _, candidate := range candidates {
		if !strings.EqualFold(candidate.ID, controller.Profile.ID) && !candidateHasHostname(candidate, profileHost) {
			continue
		}
		if candidateHasLiveLANEndpoint(candidate, localIP, targetIP, controller.Profile.PairingPort(), reachable) {
			return true, nil
		}
	}
	return false, nil
}

type reachabilityProbe func(net.IP, int, time.Duration) bool

func candidateHasLiveLANEndpoint(candidate bonjour.Candidate, localIP, targetIP net.IP, fallbackPort int, probe reachabilityProbe) bool {
	port := fallbackPort
	for _, service := range candidate.Services {
		if service.Type == "_remotepairing._tcp" {
			port = service.Port
			break
		}
	}
	for _, address := range candidate.Addresses {
		ip := net.ParseIP(address)
		if ip == nil || ip.Equal(localIP) || ip.Equal(targetIP) {
			continue
		}
		if probe(ip, port, 750*time.Millisecond) {
			return true
		}
	}
	return false
}

func candidateHasHostname(candidate bonjour.Candidate, hostname string) bool {
	for _, service := range candidate.Services {
		if strings.EqualFold(service.Hostname, hostname) {
			return true
		}
	}
	return false
}

func reachable(ip net.IP, port int, timeout time.Duration) bool {
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port)), timeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func (controller *Controller) report(status Status) {
	status.ID = controller.Profile.ID
	status.Name = controller.Profile.Name
	status.UpdatedAt = time.Now().UTC()
	if controller.Update != nil {
		controller.Update(status)
	}
}
