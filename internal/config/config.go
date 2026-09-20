package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 2

type Provider string

const (
	ProviderTailscale Provider = "tailscale"
	ProviderManual    Provider = "manual"
)

type Peer struct {
	NodeID  string `json:"nodeID,omitempty"`
	Name    string `json:"name"`
	DNSName string `json:"dnsName,omitempty"`
	Address string `json:"address"`
}

type BonjourService struct {
	Instance string   `json:"instance"`
	Type     string   `json:"type"`
	Domain   string   `json:"domain"`
	Hostname string   `json:"hostname"`
	Port     int      `json:"port"`
	TXT      []string `json:"txt,omitempty"`
}

type PortRange struct {
	First int  `json:"first"`
	Last  int  `json:"last"`
	TCP   bool `json:"tcp"`
	UDP   bool `json:"udp"`
}

type Profile struct {
	Version         int              `json:"version"`
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Provider        Provider         `json:"provider"`
	Peer            Peer             `json:"peer"`
	Interface       string           `json:"interface,omitempty"`
	Services        []BonjourService `json:"services"`
	PortRanges      []PortRange      `json:"portRanges"`
	Enabled         bool             `json:"enabled"`
	AllowLANClients bool             `json:"allowLANClients,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

func DefaultPortRanges() []PortRange {
	return []PortRange{
		{First: 52000, Last: 58000, TCP: true, UDP: true},
		{First: 61890, Last: 61930, TCP: true, UDP: true},
		{First: 62500, Last: 62531, TCP: true, UDP: true},
	}
}

func (p *Profile) Normalize() {
	if p.Version == 0 {
		p.Version = SchemaVersion
	}
	if p.Provider == "" {
		p.Provider = ProviderTailscale
	}
	if p.Interface == "" {
		p.Interface = "auto"
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	for i := range p.Services {
		p.Services[i].Type = normalizeServiceType(p.Services[i].Type)
		p.Services[i].Domain = normalizeDomain(p.Services[i].Domain)
		p.Services[i].Hostname = normalizeHostname(p.Services[i].Hostname)
		sort.Strings(p.Services[i].TXT)
	}
	p.PortRanges = MergeRanges(p.PortRanges)
}

func (p Profile) Validate() error {
	var problems []string
	if p.Version != SchemaVersion {
		problems = append(problems, fmt.Sprintf("unsupported profile version %d", p.Version))
	}
	if strings.TrimSpace(p.ID) == "" {
		problems = append(problems, "id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		problems = append(problems, "name is required")
	}
	if p.Provider != ProviderTailscale && p.Provider != ProviderManual {
		problems = append(problems, fmt.Sprintf("unsupported provider %q", p.Provider))
	}
	ip := net.ParseIP(p.Peer.Address)
	if ip == nil || ip.To4() == nil {
		problems = append(problems, "peer address must be an IPv4 address")
	}
	if len(p.Services) == 0 {
		problems = append(problems, "at least one captured Bonjour service is required")
	}
	remotePairing := false
	for i, service := range p.Services {
		prefix := fmt.Sprintf("services[%d]", i)
		if strings.TrimSpace(service.Instance) == "" {
			problems = append(problems, prefix+" instance is required")
		}
		if !validServiceType(service.Type) {
			problems = append(problems, prefix+" has an invalid service type")
		}
		if !strings.HasSuffix(service.Hostname, ".local.") {
			problems = append(problems, prefix+" hostname must end in .local.")
		}
		if service.Port < 1 || service.Port > 65535 {
			problems = append(problems, prefix+" port is invalid")
		}
		if service.Type == "_remotepairing._tcp" {
			remotePairing = true
		}
	}
	if !remotePairing {
		problems = append(problems, "a _remotepairing._tcp service is required")
	}
	for i, portRange := range p.PortRanges {
		if portRange.First < 1 || portRange.Last < portRange.First || portRange.Last > 65535 {
			problems = append(problems, fmt.Sprintf("portRanges[%d] is invalid", i))
		}
		if !portRange.TCP && !portRange.UDP {
			problems = append(problems, fmt.Sprintf("portRanges[%d] enables no protocols", i))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (p Profile) PairingPort() int {
	for _, service := range p.Services {
		if service.Type == "_remotepairing._tcp" {
			return service.Port
		}
	}
	return 49152
}

func (p Profile) Ports() (tcp []int, udp []int, err error) {
	tcpSet := map[int]bool{}
	udpSet := map[int]bool{}
	for _, service := range p.Services {
		tcpSet[service.Port] = true
	}
	for _, portRange := range p.PortRanges {
		if portRange.First < 1 || portRange.Last > 65535 || portRange.Last < portRange.First {
			return nil, nil, fmt.Errorf("invalid port range %d-%d", portRange.First, portRange.Last)
		}
		for port := portRange.First; port <= portRange.Last; port++ {
			if portRange.TCP {
				tcpSet[port] = true
			}
			if portRange.UDP {
				udpSet[port] = true
			}
		}
	}
	if len(tcpSet)+len(udpSet) > 20000 {
		return nil, nil, fmt.Errorf("profile requests too many listeners (%d); narrow the port ranges", len(tcpSet)+len(udpSet))
	}
	for port := range tcpSet {
		tcp = append(tcp, port)
	}
	for port := range udpSet {
		udp = append(udp, port)
	}
	sort.Ints(tcp)
	sort.Ints(udp)
	return tcp, udp, nil
}

func MergeRanges(ranges []PortRange) []PortRange {
	if len(ranges) < 2 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].TCP != ranges[j].TCP {
			return ranges[i].TCP
		}
		if ranges[i].UDP != ranges[j].UDP {
			return ranges[i].UDP
		}
		if ranges[i].First != ranges[j].First {
			return ranges[i].First < ranges[j].First
		}
		return ranges[i].Last < ranges[j].Last
	})
	merged := make([]PortRange, 0, len(ranges))
	for _, current := range ranges {
		last := len(merged) - 1
		if last >= 0 && merged[last].TCP == current.TCP && merged[last].UDP == current.UDP && current.First <= merged[last].Last+1 {
			if current.Last > merged[last].Last {
				merged[last].Last = current.Last
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

type Paths struct {
	Root        string
	Profiles    string
	Status      string
	Logs        string
	Executable  string
	LaunchAgent string
}

func DefaultPaths() (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	root := filepath.Join(configDir, "iPhone Tailnet Bridge")
	return Paths{
		Root:        root,
		Profiles:    filepath.Join(root, "Profiles"),
		Status:      filepath.Join(root, "status.json"),
		Logs:        filepath.Join(home, "Library", "Logs", "iPhone Tailnet Bridge"),
		Executable:  filepath.Join(home, ".local", "bin", "iphone-tailnet-bridge"),
		LaunchAgent: filepath.Join(home, "Library", "LaunchAgents", "com.github.ahmadtawakol.iphone-tailnet-bridge.plist"),
	}, nil
}

func EnsurePaths(paths Paths) error {
	for _, dir := range []string{paths.Root, paths.Profiles, paths.Logs, filepath.Dir(paths.Executable), filepath.Dir(paths.LaunchAgent)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func ProfilePath(paths Paths, id string) string {
	return filepath.Join(paths.Profiles, sanitizeID(id)+".json")
}

func WriteProfile(path string, profile Profile) error {
	profile.Normalize()
	if err := profile.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, append(data, '\n'))
}

func ReadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode %s: %w", path, err)
	}
	profile.Normalize()
	if err := profile.Validate(); err != nil {
		return Profile{}, fmt.Errorf("validate %s: %w", path, err)
	}
	return profile, nil
}

func ReadProfiles(paths Paths) ([]Profile, error) {
	entries, err := os.ReadDir(paths.Profiles)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		profile, readErr := ReadProfile(filepath.Join(paths.Profiles, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func WriteJSONPrivate(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, append(data, '\n'))
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".tailbridge-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "device"
	}
	return builder.String()
}

func normalizeServiceType(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), ".")
}

func normalizeDomain(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "local."
	}
	if !strings.HasSuffix(value, ".") {
		value += "."
	}
	return value
}

func normalizeHostname(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, ".") {
		value += "."
	}
	return value
}

func validServiceType(value string) bool {
	parts := strings.Split(strings.TrimSuffix(value, "."), ".")
	return len(parts) == 2 && strings.HasPrefix(parts[0], "_") && (parts[1] == "_tcp" || parts[1] == "_udp")
}
