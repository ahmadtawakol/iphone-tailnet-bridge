package portscan

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestOpenTCP(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ports := OpenTCP(ctx, net.ParseIP("127.0.0.1"), Options{
		First: port, Last: port, Timeout: time.Second, Concurrency: 1, Passes: 2,
	})
	if len(ports) != 1 || ports[0] != port {
		t.Fatalf("open ports = %v, want [%d]", ports, port)
	}
}

func TestFirstOpenTCP(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	found, ok := FirstOpenTCP(ctx, net.ParseIP("127.0.0.1"), Options{
		First: port - 2, Last: port + 2, Timeout: time.Second, Concurrency: 2,
	})
	if !ok || found != port {
		t.Fatalf("first open port = %d, %v; want %d, true", found, ok, port)
	}
}
