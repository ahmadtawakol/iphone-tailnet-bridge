package bonjour

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ahmadtawakol/iphone-tailnet-bridge/internal/config"
)

var CoreDeviceServiceTypes = []string{
	"_remotepairing._tcp",
	"_remoted._tcp",
	"_apple-mobdev2._tcp",
}

type Candidate struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Addresses []string                `json:"addresses,omitempty"`
	Services  []config.BonjourService `json:"services"`
}

type browseEntry struct {
	Instance  string
	Type      string
	Domain    string
	Interface int
}

var (
	browseLine  = regexp.MustCompile(`^\d\d:\d\d:\d\d\.\d+\s+Add\s+\d+\s+(\d+)\s+(\S+)\s+(\S+)\s+(.+)$`)
	resolveLine = regexp.MustCompile(`can be reached at (\S+):(\d+) \(interface (\d+)\)`)
)

func Discover(ctx context.Context, iface *net.Interface) ([]Candidate, error) {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return nil, fmt.Errorf("Apple dns-sd tool is unavailable: %w", err)
	}
	entries, browseErrors := browseAll(ctx, iface)
	if len(entries) == 0 {
		if len(browseErrors) > 0 && ctx.Err() == nil {
			return nil, fmt.Errorf("Bonjour discovery failed: %s", strings.Join(browseErrors, "; "))
		}
		return nil, nil
	}
	services, resolveErrors := resolveAll(ctx, iface, entries)
	if len(services) == 0 && len(resolveErrors) > 0 && ctx.Err() == nil {
		return nil, fmt.Errorf("Bonjour resolution failed: %s", strings.Join(resolveErrors, "; "))
	}
	return groupCandidates(services), nil
}

type resolvedService struct {
	service   config.BonjourService
	addresses []string
}

func browseAll(ctx context.Context, iface *net.Interface) ([]browseEntry, []string) {
	type result struct {
		entries []browseEntry
		err     error
	}
	results := make(chan result, len(CoreDeviceServiceTypes))
	var waitGroup sync.WaitGroup
	for _, serviceType := range CoreDeviceServiceTypes {
		serviceType := serviceType
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			entries, err := browseType(ctx, iface, serviceType)
			results <- result{entries: entries, err: err}
		}()
	}
	waitGroup.Wait()
	close(results)
	deduplicated := map[string]browseEntry{}
	var failures []string
	for result := range results {
		if result.err != nil {
			failures = append(failures, result.err.Error())
		}
		for _, entry := range result.entries {
			key := entry.Instance + "\x00" + entry.Type + "\x00" + entry.Domain
			deduplicated[key] = entry
		}
	}
	entries := make([]browseEntry, 0, len(deduplicated))
	for _, entry := range deduplicated {
		entries = append(entries, entry)
	}
	return entries, failures
}

func browseType(ctx context.Context, iface *net.Interface, serviceType string) ([]browseEntry, error) {
	arguments := []string{"-t", "2"}
	if iface != nil {
		arguments = append(arguments, "-i", iface.Name)
	}
	arguments = append(arguments, "-B", serviceType, "local.")
	command := exec.CommandContext(ctx, "/usr/bin/dns-sd", arguments...)
	output, err := command.CombinedOutput()
	entries := parseBrowseOutput(string(output))
	if len(entries) > 0 || ctx.Err() != nil {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("browse %s: %w", serviceType, err)
	}
	return entries, nil
}

func parseBrowseOutput(output string) []browseEntry {
	var entries []browseEntry
	for _, line := range strings.Split(output, "\n") {
		matches := browseLine.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) != 5 {
			continue
		}
		interfaceIndex, _ := strconv.Atoi(matches[1])
		entries = append(entries, browseEntry{
			Interface: interfaceIndex,
			Domain:    normalizeDomain(matches[2]),
			Type:      normalizeServiceType(matches[3]),
			Instance:  unescapeDNSSD(strings.TrimSpace(matches[4])),
		})
	}
	return entries
}

func resolveAll(ctx context.Context, iface *net.Interface, entries []browseEntry) ([]resolvedService, []string) {
	type result struct {
		service resolvedService
		err     error
	}
	results := make(chan result, len(entries))
	var waitGroup sync.WaitGroup
	for _, entry := range entries {
		entry := entry
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			service, err := resolveEntry(ctx, iface, entry)
			results <- result{service: service, err: err}
		}()
	}
	waitGroup.Wait()
	close(results)
	var services []resolvedService
	var failures []string
	for result := range results {
		if result.err != nil {
			failures = append(failures, result.err.Error())
			continue
		}
		services = append(services, result.service)
	}
	return services, failures
}

func resolveEntry(ctx context.Context, iface *net.Interface, entry browseEntry) (resolvedService, error) {
	arguments := []string{"-t", "2"}
	if iface != nil {
		arguments = append(arguments, "-i", iface.Name)
	}
	arguments = append(arguments, "-L", entry.Instance, entry.Type, entry.Domain)
	command := exec.CommandContext(ctx, "/usr/bin/dns-sd", arguments...)
	output, err := command.CombinedOutput()
	service, parseErr := parseResolveOutput(string(output), entry)
	if parseErr != nil {
		if ctx.Err() != nil {
			return resolvedService{}, ctx.Err()
		}
		if err != nil {
			return resolvedService{}, fmt.Errorf("resolve %s.%s: %w", entry.Instance, entry.Type, err)
		}
		return resolvedService{}, parseErr
	}
	addresses := resolveAddresses(ctx, iface, service.Hostname)
	return resolvedService{service: service, addresses: addresses}, nil
}

func parseResolveOutput(output string, entry browseEntry) (config.BonjourService, error) {
	lines := strings.Split(output, "\n")
	for index, line := range lines {
		matches := resolveLine.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}
		port, err := strconv.Atoi(matches[2])
		if err != nil {
			return config.BonjourService{}, err
		}
		var txt []string
		if index+1 < len(lines) {
			for _, value := range strings.Fields(strings.TrimSpace(lines[index+1])) {
				txt = append(txt, unescapeDNSSD(value))
			}
		}
		return config.BonjourService{
			Instance: entry.Instance,
			Type:     normalizeServiceType(entry.Type),
			Domain:   normalizeDomain(entry.Domain),
			Hostname: normalizeHostname(unescapeDNSSD(matches[1])),
			Port:     port,
			TXT:      txt,
		}, nil
	}
	return config.BonjourService{}, fmt.Errorf("dns-sd did not resolve %s.%s", entry.Instance, entry.Type)
}

func resolveAddresses(ctx context.Context, iface *net.Interface, hostname string) []string {
	arguments := []string{"-t", "1"}
	if iface != nil {
		arguments = append(arguments, "-i", iface.Name)
	}
	arguments = append(arguments, "-G", "v4", hostname)
	command := exec.CommandContext(ctx, "/usr/bin/dns-sd", arguments...)
	output, _ := command.CombinedOutput()
	seen := map[string]bool{}
	for _, field := range strings.Fields(string(output)) {
		ip := net.ParseIP(strings.TrimSpace(field))
		if ip != nil && ip.To4() != nil {
			seen[ip.String()] = true
		}
	}
	addresses := make([]string, 0, len(seen))
	for address := range seen {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses
}

func groupCandidates(services []resolvedService) []Candidate {
	type bucket struct {
		id        string
		name      string
		hostname  string
		addresses map[string]bool
		services  []config.BonjourService
	}
	buckets := map[string]*bucket{}
	for _, resolved := range services {
		service := resolved.service
		if service.Type != "_remotepairing._tcp" {
			continue
		}
		txt := parseTXT(service.TXT)
		id := firstNonEmpty(txt["identifier"], service.Instance, service.Hostname)
		key := strings.ToLower(firstNonEmpty(service.Hostname, id))
		name := firstNonEmpty(txt["name"], humanizeHostname(service.Hostname), service.Instance)
		addresses := map[string]bool{}
		for _, address := range resolved.addresses {
			addresses[address] = true
		}
		buckets[key] = &bucket{id: id, name: name, hostname: service.Hostname, addresses: addresses}
	}
	for _, resolved := range services {
		for _, current := range buckets {
			if !strings.EqualFold(resolved.service.Hostname, current.hostname) {
				continue
			}
			current.services = append(current.services, resolved.service)
			for _, address := range resolved.addresses {
				current.addresses[address] = true
			}
		}
	}
	candidates := make([]Candidate, 0, len(buckets))
	for _, current := range buckets {
		sort.Slice(current.services, func(i, j int) bool {
			if current.services[i].Type != current.services[j].Type {
				return current.services[i].Type < current.services[j].Type
			}
			return current.services[i].Port < current.services[j].Port
		})
		candidate := Candidate{ID: current.id, Name: current.name, Services: current.services}
		for address := range current.addresses {
			candidate.Addresses = append(candidate.Addresses, address)
		}
		sort.Strings(candidate.Addresses)
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return strings.ToLower(candidates[i].Name) < strings.ToLower(candidates[j].Name) })
	return candidates
}

func parseTXT(values []string) map[string]string {
	parsed := make(map[string]string, len(values))
	for _, value := range values {
		key, content, found := strings.Cut(value, "=")
		if !found {
			parsed[value] = ""
			continue
		}
		parsed[key] = content
	}
	return parsed
}

func unescapeDNSSD(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); {
		if value[index] == '\\' && index+3 < len(value) {
			if number, err := strconv.Atoi(value[index+1 : index+4]); err == nil && number >= 0 && number <= 255 {
				builder.WriteByte(byte(number))
				index += 4
				continue
			}
		}
		builder.WriteByte(value[index])
		index++
	}
	return builder.String()
}

func humanizeHostname(value string) string {
	value = strings.TrimSuffix(strings.TrimSuffix(value, "."), ".local")
	value = strings.ReplaceAll(value, "-", " ")
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
