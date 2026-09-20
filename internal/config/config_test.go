package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileRoundTripAndPrivatePermissions(t *testing.T) {
	temporary := t.TempDir()
	path := filepath.Join(temporary, "profile.json")
	profile := validProfile()
	if err := WriteProfile(path, profile); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if permission := info.Mode().Perm(); permission != 0o600 {
		t.Fatalf("profile permission = %o, want 600", permission)
	}
	decoded, err := ReadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != profile.ID || decoded.PairingPort() != 49152 {
		t.Fatalf("unexpected decoded profile: %+v", decoded)
	}
}

func TestProfileRequiresRemotePairing(t *testing.T) {
	profile := validProfile()
	profile.Services[0].Type = "_http._tcp"
	if err := profile.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPortsDeduplicateServiceAndRanges(t *testing.T) {
	profile := validProfile()
	profile.PortRanges = []PortRange{{First: 49152, Last: 49154, TCP: true, UDP: true}}
	tcp, udp, err := profile.Ports()
	if err != nil {
		t.Fatal(err)
	}
	if len(tcp) != 3 || len(udp) != 3 {
		t.Fatalf("got %d TCP and %d UDP ports", len(tcp), len(udp))
	}
}

func validProfile() Profile {
	now := time.Now().UTC()
	return Profile{
		Version:   SchemaVersion,
		ID:        "device-id",
		Name:      "Test iPhone",
		Provider:  ProviderManual,
		Peer:      Peer{Name: "phone", Address: "100.64.0.2"},
		Interface: "auto",
		Services: []BonjourService{{
			Instance: "instance",
			Type:     "_remotepairing._tcp",
			Domain:   "local.",
			Hostname: "phone.local.",
			Port:     49152,
			TXT:      []string{"identifier=device-id", "authTag=private"},
		}},
		PortRanges: []PortRange{{First: 55000, Last: 55002, TCP: true, UDP: true}},
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
