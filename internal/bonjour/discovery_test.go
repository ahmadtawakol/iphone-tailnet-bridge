package bonjour

import (
	"reflect"
	"testing"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

func TestParseBrowseOutput(t *testing.T) {
	output := `Browsing for _remotepairing._tcp.local.
Timestamp     A/R    Flags  if Domain               Service Type         Instance Name
21:44:06.290  Add        2  14 local.               _remotepairing._tcp. Tail\032Bridge\032Phone
`
	entries := parseBrowseOutput(output)
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].Instance != "Tail Bridge Phone" || entries[0].Type != "_remotepairing._tcp" || entries[0].Interface != 14 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestParseResolveOutput(t *testing.T) {
	entry := browseEntry{Instance: "Phone", Type: "_remotepairing._tcp", Domain: "local."}
	output := `Lookup Phone._remotepairing._tcp.local.
21:44:08.400  Phone._remotepairing._tcp.local. can be reached at phone.local.:49152 (interface 14)
 identifier=ABC authTag=XYZ ver=24 minVer=8 flags=0
`
	service, err := parseResolveOutput(output, entry)
	if err != nil {
		t.Fatal(err)
	}
	if service.Hostname != "phone.local." || service.Port != 49152 {
		t.Fatalf("unexpected service: %+v", service)
	}
	wantTXT := []string{"identifier=ABC", "authTag=XYZ", "ver=24", "minVer=8", "flags=0"}
	if !reflect.DeepEqual(service.TXT, wantTXT) {
		t.Fatalf("TXT = %#v, want %#v", service.TXT, wantTXT)
	}
}

func TestGroupCandidatesIncludesSiblingServices(t *testing.T) {
	services := []resolvedService{
		{service: service("_remotepairing._tcp", 49152), addresses: []string{"192.168.1.2"}},
		{service: service("_apple-mobdev2._tcp", 62078), addresses: []string{"192.168.1.2"}},
	}
	candidates := groupCandidates(services)
	if len(candidates) != 1 || len(candidates[0].Services) != 2 || candidates[0].ID != "device-id" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func service(serviceType string, port int) config.BonjourService {
	return config.BonjourService{
		Instance: "Phone",
		Type:     serviceType,
		Domain:   "local.",
		Hostname: "phone.local.",
		Port:     port,
		TXT:      []string{"identifier=device-id"},
	}
}
