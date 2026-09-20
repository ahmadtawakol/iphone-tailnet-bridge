package relay

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
	localnetwork "github.com/ahmadtawakol/iphone-tailnet-bridge/internal/network"
)

func TestRemotePairingRelayToTailnet(t *testing.T) {
	targetValue := os.Getenv("TAILBRIDGE_TARGET_IP")
	if targetValue == "" {
		t.Skip("set TAILBRIDGE_TARGET_IP to exercise a real paired iPhone")
	}
	target := net.ParseIP(targetValue)
	if target == nil || target.To4() == nil {
		t.Fatal("TAILBRIDGE_TARGET_IP is not IPv4")
	}
	local, err := localnetwork.SelectLocalInterface("auto")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(local.IPv4, target, false)
	if err != nil {
		t.Fatal(err)
	}
	profile := config.Profile{
		Services:   []config.BonjourService{{Port: 49152}},
		PortRanges: config.DefaultPortRanges(),
	}
	if err := engine.Start(profile); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort(local.IPv4.String(), "49152"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	time.Sleep(200 * time.Millisecond)
	snapshot := engine.Snapshot()
	if snapshot.TCPListeners < 6000 || snapshot.UDPListeners < 6000 {
		t.Fatalf("full listener set did not start: %+v", snapshot)
	}
	if snapshot.AcceptedTCP != 1 || snapshot.TargetDialFailures != 0 {
		t.Fatalf("unexpected relay snapshot: %+v", snapshot)
	}
}
