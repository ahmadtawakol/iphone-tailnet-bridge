package relay

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

type ListenerIssue struct {
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Message  string `json:"message"`
}

type Snapshot struct {
	TCPListeners       int             `json:"tcpListeners"`
	UDPListeners       int             `json:"udpListeners"`
	ActiveTCP          int64           `json:"activeTCP"`
	ActiveUDP          int64           `json:"activeUDP"`
	AcceptedTCP        int64           `json:"acceptedTCP"`
	RejectedClients    int64           `json:"rejectedClients"`
	TargetDialFailures int64           `json:"targetDialFailures"`
	BytesToPhone       int64           `json:"bytesToPhone"`
	BytesFromPhone     int64           `json:"bytesFromPhone"`
	Issues             []ListenerIssue `json:"issues,omitempty"`
}

type Engine struct {
	localIP         net.IP
	targetIP        net.IP
	allowLANClients bool
	targetPort      func(int) int

	context     context.Context
	cancel      context.CancelFunc
	wait        sync.WaitGroup
	mutex       sync.Mutex
	tcp         map[int]*net.TCPListener
	udp         map[int]*udpForwarder
	connections map[net.Conn]struct{}
	issues      []ListenerIssue

	activeTCP          atomic.Int64
	activeUDP          atomic.Int64
	acceptedTCP        atomic.Int64
	rejectedClients    atomic.Int64
	targetDialFailures atomic.Int64
	bytesToPhone       atomic.Int64
	bytesFromPhone     atomic.Int64
}

func New(localIP, targetIP net.IP, allowLANClients bool) (*Engine, error) {
	if localIP == nil || localIP.To4() == nil {
		return nil, fmt.Errorf("local relay IPv4 address is invalid")
	}
	if targetIP == nil || targetIP.To4() == nil {
		return nil, fmt.Errorf("target IPv4 address is invalid")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Engine{
		localIP:         localIP.To4(),
		targetIP:        targetIP.To4(),
		allowLANClients: allowLANClients,
		targetPort:      func(port int) int { return port },
		context:         ctx,
		cancel:          cancel,
		tcp:             map[int]*net.TCPListener{},
		udp:             map[int]*udpForwarder{},
		connections:     map[net.Conn]struct{}{},
	}, nil
}

func (engine *Engine) Start(profile config.Profile) error {
	tcpPorts, udpPorts, err := profile.Ports()
	if err != nil {
		return err
	}
	requiredTCP := map[int]bool{}
	for _, service := range profile.Services {
		requiredTCP[service.Port] = true
	}
	for _, port := range tcpPorts {
		listener, listenErr := net.ListenTCP("tcp4", &net.TCPAddr{IP: engine.localIP, Port: port})
		if listenErr != nil {
			issue := ListenerIssue{Protocol: "tcp", Port: port, Message: listenErr.Error()}
			engine.issues = append(engine.issues, issue)
			if requiredTCP[port] {
				engine.Stop()
				return fmt.Errorf("required TCP port %d is unavailable: %w", port, listenErr)
			}
			continue
		}
		engine.tcp[port] = listener
		engine.wait.Add(1)
		go engine.acceptTCP(port, listener)
	}
	for _, port := range udpPorts {
		forwarder, listenErr := newUDPForwarder(engine, port)
		if listenErr != nil {
			engine.issues = append(engine.issues, ListenerIssue{Protocol: "udp", Port: port, Message: listenErr.Error()})
			continue
		}
		engine.udp[port] = forwarder
		engine.wait.Add(1)
		go func() {
			defer engine.wait.Done()
			forwarder.run()
		}()
	}
	if len(engine.tcp) == 0 {
		engine.Stop()
		return fmt.Errorf("no TCP relay listeners could be started")
	}
	return nil
}

func (engine *Engine) Stop() {
	engine.cancel()
	engine.mutex.Lock()
	for _, listener := range engine.tcp {
		_ = listener.Close()
	}
	for _, forwarder := range engine.udp {
		forwarder.close()
	}
	for connection := range engine.connections {
		_ = connection.Close()
	}
	engine.mutex.Unlock()
	engine.wait.Wait()
}

func (engine *Engine) trackConnections(connections ...net.Conn) func() {
	engine.mutex.Lock()
	for _, connection := range connections {
		engine.connections[connection] = struct{}{}
	}
	engine.mutex.Unlock()
	return func() {
		engine.mutex.Lock()
		for _, connection := range connections {
			delete(engine.connections, connection)
		}
		engine.mutex.Unlock()
	}
}

func (engine *Engine) Snapshot() Snapshot {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()
	issues := append([]ListenerIssue(nil), engine.issues...)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Port != issues[j].Port {
			return issues[i].Port < issues[j].Port
		}
		return issues[i].Protocol < issues[j].Protocol
	})
	return Snapshot{
		TCPListeners:       len(engine.tcp),
		UDPListeners:       len(engine.udp),
		ActiveTCP:          engine.activeTCP.Load(),
		ActiveUDP:          engine.activeUDP.Load(),
		AcceptedTCP:        engine.acceptedTCP.Load(),
		RejectedClients:    engine.rejectedClients.Load(),
		TargetDialFailures: engine.targetDialFailures.Load(),
		BytesToPhone:       engine.bytesToPhone.Load(),
		BytesFromPhone:     engine.bytesFromPhone.Load(),
		Issues:             issues,
	}
}

func (engine *Engine) targetAddress(port int) string {
	return net.JoinHostPort(engine.targetIP.String(), fmt.Sprintf("%d", engine.targetPort(port)))
}

func (engine *Engine) clientAllowed(ip net.IP) bool {
	if engine.allowLANClients {
		return true
	}
	return ip != nil && (ip.IsLoopback() || ip.Equal(engine.localIP))
}

func (engine *Engine) dialContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(engine.context, 10*time.Second)
}
