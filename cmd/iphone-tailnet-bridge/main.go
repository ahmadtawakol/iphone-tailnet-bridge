package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/bonjour"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/bridge"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/launchagent"
	localnetwork "github.com/ahmadtawakol/iphone-tailnet-bridge/internal/network"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/portscan"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/setup"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/tailscale"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		printUsage()
		return nil
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "discover":
		return commandDiscover(arguments[1:])
	case "peers":
		return commandPeers(arguments[1:])
	case "setup":
		return commandSetup(arguments[1:], paths)
	case "import-legacy":
		return commandImportLegacy(arguments[1:], paths)
	case "doctor":
		return commandDoctor(arguments[1:], paths)
	case "run":
		return commandRun(arguments[1:], paths)
	case "serve":
		return commandServe(paths)
	case "enable":
		return commandSetEnabled(arguments[1:], paths, true)
	case "disable":
		return commandSetEnabled(arguments[1:], paths, false)
	case "status":
		return commandStatus(paths)
	case "scan":
		return commandScan(arguments[1:], paths)
	case "install":
		return commandInstall(paths)
	case "uninstall":
		return commandUninstall(paths)
	case "restart":
		return launchagent.Restart(context.Background())
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func printUsage() {
	fmt.Print(`iPhone Tailnet Bridge

Usage:
  iphone-tailnet-bridge discover          Find paired iPhones on this LAN
  iphone-tailnet-bridge peers             List iOS peers in Tailscale
  iphone-tailnet-bridge setup             Capture and configure an iPhone
  iphone-tailnet-bridge doctor            Validate a saved profile
  iphone-tailnet-bridge run               Run one profile in the foreground
  iphone-tailnet-bridge install           Install and start the LaunchAgent
  iphone-tailnet-bridge status            Show bridge status
  iphone-tailnet-bridge scan              Scan the configured phone for open ports
  iphone-tailnet-bridge enable|disable    Toggle a saved profile
  iphone-tailnet-bridge uninstall         Stop and remove the LaunchAgent
`)
}

func commandDiscover(arguments []string) error {
	flags := flag.NewFlagSet("discover", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 8*time.Second, "discovery duration")
	interfaceName := flags.String("interface", "auto", "LAN interface")
	asJSON := flags.Bool("json", false, "emit JSON")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	local, err := localnetwork.SelectLocalInterface(*interfaceName)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	candidates, err := bonjour.Discover(ctx, &local.Interface)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(os.Stdout, candidates)
	}
	if len(candidates) == 0 {
		fmt.Printf("No paired iPhone was found on %s. Keep it unlocked, on this Wi-Fi, and enable Developer Mode.\n", local.Interface.Name)
		return nil
	}
	for index, candidate := range candidates {
		fmt.Printf("%d. %s — %d CoreDevice service(s)\n", index+1, candidate.Name, len(candidate.Services))
	}
	return nil
}

func commandPeers(arguments []string) error {
	flags := flag.NewFlagSet("peers", flag.ContinueOnError)
	includeOffline := flags.Bool("all", false, "include offline iOS peers")
	asJSON := flags.Bool("json", false, "emit JSON")
	cliPath := flags.String("tailscale", "auto", "Tailscale CLI path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	client, err := tailscale.New(*cliPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peers, err := client.IOSPeers(ctx, *includeOffline)
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(os.Stdout, peers)
	}
	if len(peers) == 0 {
		fmt.Println("No iOS Tailscale peers found.")
		return nil
	}
	for index, peer := range peers {
		state := "offline"
		if peer.Online {
			state = "online"
		}
		fmt.Printf("%d. %s — %s — %s\n", index+1, peer.DisplayName(), peer.IPv4(), state)
	}
	return nil
}

func commandSetup(arguments []string, paths config.Paths) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 8*time.Second, "Bonjour discovery duration")
	deviceSelector := flags.String("device", "", "device name, identifier, or one-based index")
	peerSelector := flags.String("peer", "", "Tailscale peer name, node ID, or one-based index")
	profileName := flags.String("name", "", "profile display name")
	interfaceName := flags.String("interface", "auto", "LAN interface")
	cliPath := flags.String("tailscale", "auto", "Tailscale CLI path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if err := config.EnsurePaths(paths); err != nil {
		return err
	}
	local, err := localnetwork.SelectLocalInterface(*interfaceName)
	if err != nil {
		return err
	}
	discoveryContext, cancelDiscovery := context.WithTimeout(context.Background(), *timeout)
	candidates, err := bonjour.Discover(discoveryContext, &local.Interface)
	cancelDiscovery()
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no paired iPhone was found on %s; keep it unlocked on this Wi-Fi and rerun setup", local.Interface.Name)
	}
	candidate, err := chooseCandidate(candidates, *deviceSelector)
	if err != nil {
		return err
	}
	client, err := tailscale.New(*cliPath)
	if err != nil {
		return err
	}
	peerContext, cancelPeers := context.WithTimeout(context.Background(), 10*time.Second)
	peers, err := client.IOSPeers(peerContext, false)
	cancelPeers()
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		return fmt.Errorf("no online iOS device was found in Tailscale")
	}
	peer, err := choosePeer(peers, *peerSelector, candidate.Name)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(*profileName)
	if name == "" {
		name = candidate.Name
	}
	stableID := peer.StableID()
	if stableID == "" {
		return fmt.Errorf("the selected Tailscale peer has no stable node ID")
	}
	now := time.Now().UTC()
	profile := config.Profile{
		Version:    config.SchemaVersion,
		ID:         stableID,
		Name:       name,
		Provider:   config.ProviderTailscale,
		Peer:       config.Peer{NodeID: peer.StableID(), Name: peer.DisplayName(), DNSName: peer.DNSName, Address: peer.IPv4()},
		Interface:  *interfaceName,
		Services:   candidate.Services,
		PortRanges: config.DefaultPortRanges(),
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	profile.Normalize()
	if err := disableOtherProfiles(paths, profile.ID); err != nil {
		return err
	}
	path := config.ProfilePath(paths, profile.ID)
	if err := config.WriteProfile(path, profile); err != nil {
		return err
	}
	if err := archiveDuplicateProfiles(paths, profile); err != nil {
		return err
	}
	fmt.Printf("Saved %s\n", path)
	fmt.Printf("Matched %s to Tailscale peer %s.\n", candidate.Name, peer.DisplayName())
	return nil
}

func commandImportLegacy(arguments []string, paths config.Paths) error {
	flags := flag.NewFlagSet("import-legacy", flag.ContinueOnError)
	legacyPath := flags.String("config", "bridge.local.conf", "legacy bridge config")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	profile, err := setup.ImportLegacy(*legacyPath)
	if err != nil {
		return err
	}
	if err := config.EnsurePaths(paths); err != nil {
		return err
	}
	if err := disableOtherProfiles(paths, profile.ID); err != nil {
		return err
	}
	path := config.ProfilePath(paths, profile.ID)
	if err := config.WriteProfile(path, profile); err != nil {
		return err
	}
	fmt.Printf("Imported legacy profile to %s\n", path)
	return nil
}

func commandDoctor(arguments []string, paths config.Paths) error {
	profile, _, err := profileFromFlags("doctor", arguments, paths)
	if err != nil {
		return err
	}
	local, err := localnetwork.SelectLocalInterface(profile.Interface)
	if err != nil {
		return err
	}
	tcpPorts, udpPorts, err := profile.Ports()
	if err != nil {
		return err
	}
	target, online, err := resolveProfilePeer(profile)
	if err != nil {
		return err
	}
	fmt.Printf("Profile: %s\n", profile.Name)
	fmt.Printf("LAN interface: %s (%s)\n", local.Interface.Name, local.IPv4)
	fmt.Printf("iPhone target: %s (%s)\n", profile.Peer.Name, target)
	fmt.Printf("Relay listeners: %d TCP, %d UDP in one process\n", len(tcpPorts), len(udpPorts))
	if !online {
		return fmt.Errorf("the iPhone is offline in Tailscale")
	}
	if !portReachable(target, profile.PairingPort(), 3*time.Second) {
		return fmt.Errorf("RemotePairing port %d is not reachable; the iPhone must be on Wi-Fi with Tailscale connected", profile.PairingPort())
	}
	fmt.Printf("RemotePairing port %d: reachable\n", profile.PairingPort())
	fmt.Println("Doctor passed.")
	return nil
}

func commandRun(arguments []string, paths config.Paths) error {
	profile, _, err := profileFromFlags("run", arguments, paths)
	if err != nil {
		return err
	}
	client, clientErr := tailscale.New("auto")
	if profile.Provider == config.ProviderTailscale && clientErr != nil {
		return clientErr
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	controller := bridge.Controller{Profile: profile, Tailscale: client, Update: func(status bridge.Status) {
		fmt.Printf("%s: %s\n", status.State, status.Message)
	}}
	return controller.Run(ctx)
}

func commandServe(paths config.Paths) error {
	if err := config.EnsurePaths(paths); err != nil {
		return err
	}
	profiles, err := config.ReadProfiles(paths)
	if err != nil {
		return err
	}
	var client *tailscale.Client
	if os.Getenv("XPC_SERVICE_NAME") == "" {
		client, _ = tailscale.New("auto")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service := bridge.Service{Paths: paths, Tailscale: client}
	return service.Run(ctx, profiles)
}

func commandSetEnabled(arguments []string, paths config.Paths, enabled bool) error {
	profile, path, err := profileFromFlags("toggle", arguments, paths)
	if err != nil {
		return err
	}
	if enabled {
		if err := disableOtherProfiles(paths, profile.ID); err != nil {
			return err
		}
	}
	profile.Enabled = enabled
	if err := config.WriteProfile(path, profile); err != nil {
		return err
	}
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("%s: %s\n", profile.Name, state)
	return nil
}

func commandStatus(paths config.Paths) error {
	data, err := os.ReadFile(paths.Status)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Bridge has not reported status yet.")
			return nil
		}
		return err
	}
	var document bridge.StatusDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	if len(document.Devices) == 0 {
		fmt.Println("No configured devices.")
		return nil
	}
	for _, device := range document.Devices {
		fmt.Printf("%s: %s — %s\n", device.Name, device.State.DisplayName(), device.Message)
		if device.State == bridge.StateActive || device.State == bridge.StateDegraded {
			fmt.Printf("  %d TCP + %d UDP listeners; %d active connection(s)\n", device.Relay.TCPListeners, device.Relay.UDPListeners, device.Relay.ActiveTCP+device.Relay.ActiveUDP)
		}
	}
	return nil
}

func commandScan(arguments []string, paths config.Paths) error {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	profilePath := flags.String("profile", "", "profile path")
	first := flags.Int("first", 49152, "first port")
	last := flags.Int("last", 65535, "last port")
	timeout := flags.Duration("timeout", 350*time.Millisecond, "per-port timeout")
	concurrency := flags.Int("concurrency", 256, "parallel probes")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	profile, _, err := loadSelectedProfile(paths, *profilePath)
	if err != nil {
		return err
	}
	if *first < 1 || *last < *first || *last > 65535 || *concurrency < 1 || *concurrency > 2048 {
		return fmt.Errorf("invalid scan bounds")
	}
	target, online, err := resolveProfilePeer(profile)
	if err != nil {
		return err
	}
	if !online {
		return fmt.Errorf("the iPhone is offline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	open := portscan.OpenTCP(ctx, target, portscan.Options{
		First:       *first,
		Last:        *last,
		Timeout:     *timeout,
		Concurrency: *concurrency,
		Passes:      2,
	})
	if len(open) == 0 {
		fmt.Println("No open TCP ports found in the selected range.")
		return nil
	}
	fmt.Print("Open TCP ports:")
	for _, port := range open {
		fmt.Printf(" %d", port)
	}
	fmt.Println()
	return nil
}

func commandInstall(paths config.Paths) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := launchagent.Install(context.Background(), paths, executable); err != nil {
		return err
	}
	fmt.Printf("Installed %s and started the bridge.\n", paths.Executable)
	return nil
}

func commandUninstall(paths config.Paths) error {
	if err := launchagent.Remove(context.Background(), paths); err != nil {
		return err
	}
	fmt.Println("Stopped and removed the LaunchAgent. Device profiles were preserved.")
	return nil
}

func profileFromFlags(name string, arguments []string, paths config.Paths) (config.Profile, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	profilePath := flags.String("profile", "", "profile path")
	if err := flags.Parse(arguments); err != nil {
		return config.Profile{}, "", err
	}
	return loadSelectedProfile(paths, *profilePath)
}

func loadSelectedProfile(paths config.Paths, path string) (config.Profile, string, error) {
	if path != "" {
		profile, err := config.ReadProfile(path)
		return profile, path, err
	}
	profiles, err := config.ReadProfiles(paths)
	if err != nil {
		return config.Profile{}, "", err
	}
	if len(profiles) == 0 {
		return config.Profile{}, "", fmt.Errorf("no device profile exists; run setup first")
	}
	selected := profiles[0]
	for _, profile := range profiles {
		if profile.Enabled {
			selected = profile
			break
		}
	}
	return selected, config.ProfilePath(paths, selected.ID), nil
}

func disableOtherProfiles(paths config.Paths, keepID string) error {
	profiles, err := config.ReadProfiles(paths)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == keepID || !profile.Enabled {
			continue
		}
		profile.Enabled = false
		if err := config.WriteProfile(config.ProfilePath(paths, profile.ID), profile); err != nil {
			return err
		}
	}
	return nil
}

func archiveDuplicateProfiles(paths config.Paths, keep config.Profile) error {
	profiles, err := config.ReadProfiles(paths)
	if err != nil {
		return err
	}
	archiveDirectory := filepath.Join(paths.Profiles, "Archive")
	for _, profile := range profiles {
		if profile.ID == keep.ID || !samePeer(profile.Peer, keep.Peer) {
			continue
		}
		if err := os.MkdirAll(archiveDirectory, 0o700); err != nil {
			return err
		}
		source := config.ProfilePath(paths, profile.ID)
		destination := filepath.Join(archiveDirectory, time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+filepath.Base(source))
		if err := os.Rename(source, destination); err != nil {
			return fmt.Errorf("archive duplicate profile %s: %w", profile.Name, err)
		}
	}
	return nil
}

func samePeer(left, right config.Peer) bool {
	if left.NodeID != "" && right.NodeID != "" {
		return left.NodeID == right.NodeID
	}
	if left.DNSName != "" && right.DNSName != "" {
		return strings.EqualFold(left.DNSName, right.DNSName)
	}
	return left.Address != "" && left.Address == right.Address
}

func chooseCandidate(candidates []bonjour.Candidate, selector string) (bonjour.Candidate, error) {
	if candidate, ok := matchCandidate(candidates, selector); ok {
		return candidate, nil
	}
	if selector != "" {
		return bonjour.Candidate{}, fmt.Errorf("no discovered device matches %q", selector)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	fmt.Println("Discovered paired iPhones:")
	for index, candidate := range candidates {
		fmt.Printf("  %d. %s\n", index+1, candidate.Name)
	}
	choice, err := promptIndex(len(candidates), "Choose the iPhone")
	if err != nil {
		return bonjour.Candidate{}, err
	}
	return candidates[choice], nil
}

func matchCandidate(candidates []bonjour.Candidate, selector string) (bonjour.Candidate, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return bonjour.Candidate{}, false
	}
	if index, err := strconv.Atoi(selector); err == nil && index > 0 && index <= len(candidates) {
		return candidates[index-1], true
	}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.ID, selector) || strings.EqualFold(candidate.Name, selector) || strings.Contains(strings.ToLower(candidate.Name), strings.ToLower(selector)) {
			return candidate, true
		}
	}
	return bonjour.Candidate{}, false
}

func choosePeer(peers []tailscale.Peer, selector, hint string) (tailscale.Peer, error) {
	if peer, ok := matchPeer(peers, selector); ok {
		return peer, nil
	}
	if selector != "" {
		return tailscale.Peer{}, fmt.Errorf("no online iOS Tailscale peer matches %q", selector)
	}
	for _, peer := range peers {
		if normalizedName(peer.DisplayName()) == normalizedName(hint) {
			return peer, nil
		}
	}
	if len(peers) == 1 {
		return peers[0], nil
	}
	fmt.Println("Online iOS devices in Tailscale:")
	for index, peer := range peers {
		fmt.Printf("  %d. %s (%s)\n", index+1, peer.DisplayName(), peer.IPv4())
	}
	choice, err := promptIndex(len(peers), "Choose the matching Tailscale device")
	if err != nil {
		return tailscale.Peer{}, err
	}
	return peers[choice], nil
}

func matchPeer(peers []tailscale.Peer, selector string) (tailscale.Peer, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return tailscale.Peer{}, false
	}
	if index, err := strconv.Atoi(selector); err == nil && index > 0 && index <= len(peers) {
		return peers[index-1], true
	}
	for _, peer := range peers {
		if strings.EqualFold(peer.StableID(), selector) || strings.EqualFold(peer.DisplayName(), selector) || strings.EqualFold(strings.TrimSuffix(peer.DNSName, "."), strings.TrimSuffix(selector, ".")) {
			return peer, true
		}
	}
	return tailscale.Peer{}, false
}

func promptIndex(count int, label string) (int, error) {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return 0, fmt.Errorf("multiple choices are available; rerun with an explicit selector")
	}
	fmt.Printf("%s [1-%d]: ", label, count)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return 0, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > count {
		return 0, fmt.Errorf("invalid selection")
	}
	return choice - 1, nil
}

func normalizedName(value string) string {
	value = strings.ToLower(value)
	value = strings.NewReplacer("-", "", "_", "", " ", "", "’", "", "'", "").Replace(value)
	return value
}

func resolveProfilePeer(profile config.Profile) (net.IP, bool, error) {
	if profile.Provider == config.ProviderManual {
		ip := net.ParseIP(profile.Peer.Address)
		if ip == nil || ip.To4() == nil {
			return nil, false, fmt.Errorf("invalid target address")
		}
		return ip.To4(), true, nil
	}
	client, err := tailscale.New("auto")
	if err != nil {
		return nil, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peer, err := client.Resolve(ctx, profile.Peer.NodeID, profile.Peer.Name)
	if err != nil {
		return nil, false, err
	}
	ip := net.ParseIP(peer.IPv4())
	if ip == nil {
		return nil, false, fmt.Errorf("peer has no IPv4 address")
	}
	return ip.To4(), peer.Online, nil
}

func portReachable(ip net.IP, port int, timeout time.Duration) bool {
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(ip.String(), strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func writeJSON(file *os.File, value any) error {
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
