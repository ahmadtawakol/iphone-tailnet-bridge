package setup

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

func ImportLegacy(path string) (config.Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return config.Profile{}, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return config.Profile{}, err
	}
	required := []string{"IPHONE_TAILNET_IP", "IPHONE_HOSTNAME", "INSTANCE_UUID", "IDENTIFIER", "AUTH_TAG", "VER", "MIN_VER", "FLAGS", "PAIRING_PORT"}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" || strings.Contains(values[key], "REPLACE_ME") {
			return config.Profile{}, fmt.Errorf("legacy config is missing %s", key)
		}
	}
	pairingPort, err := strconv.Atoi(values["PAIRING_PORT"])
	if err != nil {
		return config.Profile{}, fmt.Errorf("invalid PAIRING_PORT: %w", err)
	}
	txt := []string{
		"identifier=" + values["IDENTIFIER"],
		"authTag=" + values["AUTH_TAG"],
		"ver=" + values["VER"],
		"minVer=" + values["MIN_VER"],
		"flags=" + values["FLAGS"],
	}
	services := make([]config.BonjourService, 0, 3)
	for _, serviceType := range []string{"_remotepairing._tcp", "_remoted._tcp", "_apple-mobdev2._tcp"} {
		services = append(services, config.BonjourService{
			Instance: values["INSTANCE_UUID"],
			Type:     serviceType,
			Domain:   "local.",
			Hostname: values["IPHONE_HOSTNAME"],
			Port:     pairingPort,
			TXT:      append([]string(nil), txt...),
		})
	}
	ranges := rangesFromLegacy(values)
	now := time.Now().UTC()
	profile := config.Profile{
		Version:    config.SchemaVersion,
		ID:         values["INSTANCE_UUID"],
		Name:       humanize(values["IPHONE_HOSTNAME"]),
		Provider:   config.ProviderManual,
		Peer:       config.Peer{Name: humanize(values["IPHONE_HOSTNAME"]), Address: values["IPHONE_TAILNET_IP"]},
		Interface:  firstNonEmpty(values["INTERFACE"], "auto"),
		Services:   services,
		PortRanges: ranges,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	profile.Normalize()
	return profile, profile.Validate()
}

func rangesFromLegacy(values map[string]string) []config.PortRange {
	keys := [][2]string{{"PORT_FIRST", "PORT_LAST"}, {"EXTRA_PORT_FIRST", "EXTRA_PORT_LAST"}, {"TUNNEL_PORT_FIRST", "TUNNEL_PORT_LAST"}}
	var ranges []config.PortRange
	for _, pair := range keys {
		first, firstErr := strconv.Atoi(values[pair[0]])
		last, lastErr := strconv.Atoi(values[pair[1]])
		if firstErr == nil && lastErr == nil && first > 0 && last >= first {
			ranges = append(ranges, config.PortRange{First: first, Last: last, TCP: true, UDP: true})
		}
	}
	if len(ranges) == 0 {
		return config.DefaultPortRanges()
	}
	return ranges
}

func humanize(value string) string {
	value = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "."), ".local")
	return strings.ReplaceAll(value, "-", " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
