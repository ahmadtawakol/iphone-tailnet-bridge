package relay

import (
	"errors"
	"io"
	"net"
	"time"
)

func (engine *Engine) acceptTCP(port int, listener *net.TCPListener) {
	defer engine.wait.Done()
	for {
		connection, err := listener.AcceptTCP()
		if err != nil {
			if engine.context.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		remoteAddress, _ := connection.RemoteAddr().(*net.TCPAddr)
		if remoteAddress == nil || !engine.clientAllowed(remoteAddress.IP) {
			engine.rejectedClients.Add(1)
			_ = connection.Close()
			continue
		}
		engine.acceptedTCP.Add(1)
		engine.activeTCP.Add(1)
		engine.wait.Add(1)
		go func() {
			defer engine.wait.Done()
			defer engine.activeTCP.Add(-1)
			engine.forwardTCP(port, connection)
		}()
	}
}

func (engine *Engine) forwardTCP(port int, local *net.TCPConn) {
	defer local.Close()
	_ = local.SetKeepAlive(true)
	ctx, cancel := engine.dialContext()
	defer cancel()
	dialer := net.Dialer{KeepAlive: 30 * time.Second}
	remoteConnection, err := dialer.DialContext(ctx, "tcp4", engine.targetAddress(port))
	if err != nil {
		engine.targetDialFailures.Add(1)
		return
	}
	remote, ok := remoteConnection.(*net.TCPConn)
	if !ok {
		remoteConnection.Close()
		engine.targetDialFailures.Add(1)
		return
	}
	defer remote.Close()
	_ = remote.SetKeepAlive(true)
	untrack := engine.trackConnections(local, remote)
	defer untrack()
	if engine.context.Err() != nil {
		return
	}

	type copyResult struct {
		toPhone bool
		bytes   int64
	}
	results := make(chan copyResult, 2)
	go func() {
		count, _ := io.Copy(remote, local)
		_ = remote.CloseWrite()
		results <- copyResult{toPhone: true, bytes: count}
	}()
	go func() {
		count, _ := io.Copy(local, remote)
		_ = local.CloseWrite()
		results <- copyResult{toPhone: false, bytes: count}
	}()
	for index := 0; index < 2; index++ {
		result := <-results
		if result.toPhone {
			engine.bytesToPhone.Add(result.bytes)
		} else {
			engine.bytesFromPhone.Add(result.bytes)
		}
	}
}
