package tailscale

import "testing"

func TestDecodeStatusAcceptsNumericNodeIDAndDiagnosticPrefix(t *testing.T) {
	output := []byte("non-fatal diagnostic\n{\"Peer\":{\"node\":{\"ID\":\"stable-id\",\"NodeID\":1234,\"HostName\":\"iPhone\",\"DNSName\":\"iphone.example.ts.net.\",\"OS\":\"iOS\",\"Online\":true,\"TailscaleIPs\":[\"100.64.0.2\"]}}}")
	document, err := decodeStatus(output)
	if err != nil {
		t.Fatal(err)
	}
	peer := document.Peer["node"]
	if peer.NodeID != 1234 || peer.StableID() != "stable-id" || peer.IPv4() != "100.64.0.2" {
		t.Fatalf("unexpected peer: %+v", peer)
	}
}

func TestDecodeStatusRejectsMissingJSON(t *testing.T) {
	if _, err := decodeStatus([]byte("GUI unavailable")); err == nil {
		t.Fatal("expected an error")
	}
}
