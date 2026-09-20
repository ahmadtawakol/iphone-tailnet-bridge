package relay

import (
	"errors"
	"net"
	"sync"
	"time"
)

type udpForwarder struct {
	engine   *Engine
	port     int
	listener *net.UDPConn
	mutex    sync.Mutex
	sessions map[string]*udpSession
}

type udpSession struct {
	forwarder *udpForwarder
	client    *net.UDPAddr
	remote    *net.UDPConn
	lastSeen  time.Time
	closed    chan struct{}
}

func newUDPForwarder(engine *Engine, port int) (*udpForwarder, error) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: engine.localIP, Port: port})
	if err != nil {
		return nil, err
	}
	return &udpForwarder{engine: engine, port: port, listener: listener, sessions: map[string]*udpSession{}}, nil
}

func (forwarder *udpForwarder) run() {
	buffer := make([]byte, 65535)
	for {
		_ = forwarder.listener.SetReadDeadline(time.Now().Add(time.Second))
		count, client, err := forwarder.listener.ReadFromUDP(buffer)
		if err != nil {
			if forwarder.engine.context.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				forwarder.expireSessions()
				continue
			}
			continue
		}
		if client == nil || !forwarder.engine.clientAllowed(client.IP) {
			forwarder.engine.rejectedClients.Add(1)
			continue
		}
		session, err := forwarder.session(client)
		if err != nil {
			forwarder.engine.targetDialFailures.Add(1)
			continue
		}
		payload := append([]byte(nil), buffer[:count]...)
		written, err := session.remote.Write(payload)
		if err != nil {
			session.shutdown()
			continue
		}
		forwarder.engine.bytesToPhone.Add(int64(written))
	}
}

func (forwarder *udpForwarder) session(client *net.UDPAddr) (*udpSession, error) {
	key := client.String()
	forwarder.mutex.Lock()
	defer forwarder.mutex.Unlock()
	if existing := forwarder.sessions[key]; existing != nil {
		existing.lastSeen = time.Now()
		return existing, nil
	}
	target := &net.UDPAddr{IP: forwarder.engine.targetIP, Port: forwarder.engine.targetPort(forwarder.port)}
	remote, err := net.DialUDP("udp4", nil, target)
	if err != nil {
		return nil, err
	}
	session := &udpSession{
		forwarder: forwarder,
		client:    client,
		remote:    remote,
		lastSeen:  time.Now(),
		closed:    make(chan struct{}),
	}
	forwarder.sessions[key] = session
	forwarder.engine.activeUDP.Add(1)
	forwarder.engine.wait.Add(1)
	go session.readReplies()
	return session, nil
}

func (session *udpSession) readReplies() {
	defer session.forwarder.engine.wait.Done()
	defer session.remove()
	buffer := make([]byte, 65535)
	for {
		_ = session.remote.SetReadDeadline(time.Now().Add(time.Second))
		count, err := session.remote.Read(buffer)
		if err != nil {
			if session.forwarder.engine.context.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				select {
				case <-session.closed:
					return
				default:
					continue
				}
			}
			return
		}
		written, err := session.forwarder.listener.WriteToUDP(buffer[:count], session.client)
		if err != nil {
			return
		}
		session.forwarder.engine.bytesFromPhone.Add(int64(written))
	}
}

func (forwarder *udpForwarder) expireSessions() {
	forwarder.mutex.Lock()
	defer forwarder.mutex.Unlock()
	cutoff := time.Now().Add(-2 * time.Minute)
	for key, session := range forwarder.sessions {
		if session.lastSeen.Before(cutoff) {
			delete(forwarder.sessions, key)
			session.shutdownLocked()
		}
	}
}

func (session *udpSession) remove() {
	forwarder := session.forwarder
	forwarder.mutex.Lock()
	defer forwarder.mutex.Unlock()
	key := session.client.String()
	if forwarder.sessions[key] == session {
		delete(forwarder.sessions, key)
	}
	session.shutdownLocked()
}

func (session *udpSession) shutdown() {
	session.forwarder.mutex.Lock()
	defer session.forwarder.mutex.Unlock()
	session.shutdownLocked()
}

func (session *udpSession) shutdownLocked() {
	select {
	case <-session.closed:
		return
	default:
		close(session.closed)
		_ = session.remote.Close()
		session.forwarder.engine.activeUDP.Add(-1)
	}
}

func (forwarder *udpForwarder) close() {
	_ = forwarder.listener.Close()
	forwarder.mutex.Lock()
	defer forwarder.mutex.Unlock()
	for key, session := range forwarder.sessions {
		delete(forwarder.sessions, key)
		session.shutdownLocked()
	}
}
