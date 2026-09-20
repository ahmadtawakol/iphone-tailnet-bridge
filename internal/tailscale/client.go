package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Peer struct {
	ID           string    `json:"ID"`
	NodeID       uint64    `json:"NodeID"`
	HostName     string    `json:"HostName"`
	DNSName      string    `json:"DNSName"`
	OS           string    `json:"OS"`
	Online       bool      `json:"Online"`
	Active       bool      `json:"Active"`
	TailscaleIPs []string  `json:"TailscaleIPs"`
	LastSeen     time.Time `json:"LastSeen"`
}

type statusDocument struct {
	Peer map[string]Peer `json:"Peer"`
}

type Client struct {
	Path string
}

func New(path string) (*Client, error) {
	if path == "" || path == "auto" {
		path = findCLI()
	}
	if path == "" {
		return nil, errors.New("Tailscale CLI was not found; install Tailscale or provide its path")
	}
	return &Client{Path: path}, nil
}

func findCLI() string {
	if path, err := exec.LookPath("tailscale"); err == nil {
		return path
	}
	candidates := []string{
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
		"/usr/local/bin/tailscale",
		"/opt/homebrew/bin/tailscale",
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Applications", "Tailscale.app", "Contents", "MacOS", "Tailscale"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (client *Client) Peers(ctx context.Context) ([]Peer, error) {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" && os.Getenv("XPC_SERVICE_NAME") != "" {
		command = exec.CommandContext(ctx, "/bin/launchctl", "asuser", strconv.Itoa(os.Getuid()), client.Path, "status", "--json")
	} else {
		command = exec.CommandContext(ctx, client.Path, "status", "--json")
	}
	command.Env = scrubLaunchdEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("tailscale status: %s", message)
	}
	document, err := decodeStatus(output)
	if err != nil {
		return nil, err
	}
	peers := make([]Peer, 0, len(document.Peer))
	for _, peer := range document.Peer {
		peers = append(peers, peer)
	}
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].Online != peers[j].Online {
			return peers[i].Online
		}
		return strings.ToLower(peers[i].DisplayName()) < strings.ToLower(peers[j].DisplayName())
	})
	return peers, nil
}

func decodeStatus(output []byte) (statusDocument, error) {
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart < 0 {
		sample := strings.TrimSpace(string(output))
		if len(sample) > 200 {
			sample = sample[:200] + "…"
		}
		if sample == "" {
			return statusDocument{}, fmt.Errorf("tailscale status returned no JSON and no diagnostic output")
		}
		return statusDocument{}, fmt.Errorf("tailscale status did not return JSON: %q", sample)
	}
	var document statusDocument
	if err := json.NewDecoder(bytes.NewReader(output[jsonStart:])).Decode(&document); err != nil {
		return statusDocument{}, fmt.Errorf("decode tailscale status: %w", err)
	}
	return document, nil
}

func scrubLaunchdEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		switch key {
		case "XPC_SERVICE_NAME", "XPC_FLAGS", "OSLogRateLimit", "CFProcessPath":
			continue
		default:
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func (client *Client) IOSPeers(ctx context.Context, includeOffline bool) ([]Peer, error) {
	peers, err := client.Peers(ctx)
	if err != nil {
		return nil, err
	}
	filtered := peers[:0]
	for _, peer := range peers {
		if !strings.EqualFold(peer.OS, "iOS") && !strings.EqualFold(peer.OS, "iPadOS") {
			continue
		}
		if !includeOffline && !peer.Online {
			continue
		}
		if peer.IPv4() == "" {
			continue
		}
		filtered = append(filtered, peer)
	}
	return filtered, nil
}

func (client *Client) Resolve(ctx context.Context, nodeID, name string) (Peer, error) {
	peers, err := client.Peers(ctx)
	if err != nil {
		return Peer{}, err
	}
	for _, peer := range peers {
		if nodeID != "" && peer.StableID() == nodeID {
			return peer, nil
		}
	}
	for _, peer := range peers {
		if strings.EqualFold(peer.HostName, name) || strings.EqualFold(strings.TrimSuffix(peer.DNSName, "."), strings.TrimSuffix(name, ".")) {
			return peer, nil
		}
	}
	return Peer{}, fmt.Errorf("Tailscale peer %q was not found", name)
}

func (peer Peer) IPv4() string {
	for _, value := range peer.TailscaleIPs {
		ip := net.ParseIP(value)
		if ip != nil && ip.To4() != nil {
			return ip.String()
		}
	}
	return ""
}

func (peer Peer) DisplayName() string {
	if peer.HostName != "" && !strings.EqualFold(peer.HostName, "localhost") {
		return peer.HostName
	}
	if peer.DNSName != "" {
		return strings.Split(strings.TrimSuffix(peer.DNSName, "."), ".")[0]
	}
	return peer.StableID()
}

func (peer Peer) StableID() string {
	if peer.ID != "" {
		return peer.ID
	}
	if peer.NodeID != 0 {
		return strconv.FormatUint(peer.NodeID, 10)
	}
	return ""
}
