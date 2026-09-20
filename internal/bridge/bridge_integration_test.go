package bridge

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

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
