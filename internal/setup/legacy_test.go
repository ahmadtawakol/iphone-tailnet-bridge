package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge.local.conf")
	content := `IPHONE_TAILNET_IP='100.64.0.2'
IPHONE_HOSTNAME='Phone.local'
INSTANCE_UUID='instance-id'
IDENTIFIER='identifier-id'
AUTH_TAG='private-tag'
VER='24'
MIN_VER='8'
FLAGS='0'
PAIRING_PORT=49152
PORT_FIRST=55000
PORT_LAST=55002
INTERFACE=en0
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := ImportLegacy(path)
	if err != nil {
		t.Fatal(err)
	}
	if profile.PairingPort() != 49152 || len(profile.Services) != 3 || profile.Peer.Address != "100.64.0.2" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}
