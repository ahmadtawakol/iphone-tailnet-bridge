package bridge

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/bonjour"
	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

func TestCachedBonjourRecordMustHaveLiveLANEndpoint(t *testing.T) {
	candidate := bonjour.Candidate{
		Addresses: []string{"192.168.1.44"},
		Services:  []config.BonjourService{{Type: "_remotepairing._tcp", Port: 49152}},
	}
	local := net.ParseIP("192.168.1.25")
	target := net.ParseIP("100.64.0.2")
	if candidateHasLiveLANEndpoint(candidate, local, target, 49152, func(net.IP, int, time.Duration) bool { return false }) {
		t.Fatal("stale Bonjour cache was treated as a live local device")
	}
	if !candidateHasLiveLANEndpoint(candidate, local, target, 49152, func(ip net.IP, port int, _ time.Duration) bool {
		return ip.String() == "192.168.1.44" && port == 49152
	}) {
		t.Fatal("live LAN endpoint was not recognized")
	}
}

func TestRefreshesDynamicMobdevPort(t *testing.T) {
	targetValue := os.Getenv("TAILBRIDGE_TARGET_IP")
	if targetValue == "" {
		t.Skip("set TAILBRIDGE_TARGET_IP to exercise a real paired iPhone")
	}
	target := net.ParseIP(targetValue)
	if target == nil || target.To4() == nil {
		t.Fatal("TAILBRIDGE_TARGET_IP is not IPv4")
	}
	controller := Controller{Profile: config.Profile{Services: []config.BonjourService{
		{Type: "_remotepairing._tcp", Port: 49152},
		{Type: "_apple-mobdev2._tcp", Port: 30000},
	}}}
	profile, adjusted := controller.refreshServicePorts(context.Background(), target)
	if adjusted != 1 || profile.Services[1].Port == 30000 {
		t.Fatalf("dynamic service was not refreshed: adjusted=%d port=%d", adjusted, profile.Services[1].Port)
	}
}
