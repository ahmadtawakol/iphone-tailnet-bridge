package relay

import (
	"bytes"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

func TestTCPRelayRoundTrip(t *testing.T) {
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetPort := target.Addr().(*net.TCPAddr).Port
	port := availableTCPPort(t)
	go func() {
		connection, acceptErr := target.AcceptTCP()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = io.Copy(connection, connection)
	}()

	engine, err := New(net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.1"), false)
	if err != nil {
		t.Fatal(err)
	}
	engine.targetPort = func(int) int { return targetPort }
	profile := relayProfile(port, true, false)
	if err := engine.Start(profile); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("tailbridge tcp")
	if _, err := connection.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(connection, response); err != nil {
		t.Fatal(err)
	}
	connection.Close()
	if !bytes.Equal(response, payload) {
		t.Fatalf("response %q != %q", response, payload)
	}
}

func TestUDPRelayRoundTrip(t *testing.T) {
	target, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetPort := target.LocalAddr().(*net.UDPAddr).Port
	port := availableUDPPort(t)
	go func() {
		buffer := make([]byte, 2048)
		count, address, readErr := target.ReadFromUDP(buffer)
		if readErr == nil {
			_, _ = target.WriteToUDP(buffer[:count], address)
		}
	}()

	engine, err := New(net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.1"), false)
	if err != nil {
		t.Fatal(err)
	}
	engine.targetPort = func(int) int { return targetPort }
	profile := relayProfile(port, false, true)
	if err := engine.Start(profile); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	connection, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	payload := []byte("tailbridge udp")
	if _, err := connection.Write(payload); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 2048)
	count, err := connection.Read(response)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response[:count], payload) {
		t.Fatalf("response %q != %q", response[:count], payload)
	}
}

func TestStopClosesActiveTCPConnections(t *testing.T) {
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	targetPort := target.Addr().(*net.TCPAddr).Port
	targetAccepted := make(chan *net.TCPConn, 1)
	go func() {
		connection, acceptErr := target.AcceptTCP()
		if acceptErr == nil {
			targetAccepted <- connection
		}
	}()
	port := availableTCPPort(t)
	engine, err := New(net.ParseIP("127.0.0.1"), net.ParseIP("127.0.0.1"), false)
	if err != nil {
		t.Fatal(err)
	}
	engine.targetPort = func(int) int { return targetPort }
	if err := engine.Start(relayProfile(port, true, false)); err != nil {
		t.Fatal(err)
	}
	local, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	var remote *net.TCPConn
	select {
	case remote = <-targetAccepted:
		defer remote.Close()
	case <-time.After(time.Second):
		t.Fatal("target did not accept the relayed connection")
	}
	stopped := make(chan struct{})
	go func() {
		engine.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Engine.Stop blocked on an active TCP connection")
	}
}

func relayProfile(port int, tcp, udp bool) config.Profile {
	return config.Profile{
		Services: []config.BonjourService{{Port: port}},
		PortRanges: []config.PortRange{{
			First: port,
			Last:  port,
			TCP:   tcp,
			UDP:   udp,
		}},
	}
}

func availableTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}

func availableUDPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := listener.LocalAddr().(*net.UDPAddr).Port
	listener.Close()
	return port
}
