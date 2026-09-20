package bonjour

import (
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
	localnetwork "github.com/ahmadtawakol/iphone-tailnet-bridge/internal/network"
)

func TestPublisherVisibleToSystemBonjour(t *testing.T) {
	if os.Getenv("TAILBRIDGE_INTEGRATION") == "" {
		t.Skip("set TAILBRIDGE_INTEGRATION=1 to exercise system Bonjour")
	}
	local, err := localnetwork.SelectLocalInterface("auto")
	if err != nil {
		t.Fatal(err)
	}
	service := config.BonjourService{
		Instance: "Tailbridge Integration Test",
		Type:     "_tailbridge-test._tcp",
		Domain:   "local.",
		Hostname: "tailbridge-integration.local.",
		Port:     49152,
		TXT:      []string{"marker=tailbridge"},
	}
	publisher := &Publisher{}
	if err := publisher.Start([]config.BonjourService{service}, local.Interface, local.IPv4); err != nil {
		t.Fatal(err)
	}
	defer publisher.Stop()
	command := exec.Command("/usr/bin/dns-sd", "-t", "2", "-i", local.Interface.Name, "-L", service.Instance, service.Type, service.Domain)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("dns-sd lookup: %v\n%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "marker=tailbridge") {
		t.Fatalf("published record not found:\n%s", text)
	}
	addressCommand := exec.Command("/usr/bin/dns-sd", "-t", "2", "-i", local.Interface.Name, "-G", "v4", service.Hostname)
	addressOutput, err := addressCommand.CombinedOutput()
	if err != nil || !strings.Contains(string(addressOutput), net.IP(local.IPv4).String()) {
		t.Fatalf("published address not found: %v\n%s", err, addressOutput)
	}
}
