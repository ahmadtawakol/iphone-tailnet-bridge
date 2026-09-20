package network

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

type LocalInterface struct {
	Interface net.Interface
	IPv4      net.IP
}

func SelectLocalInterface(requested string) (LocalInterface, error) {
	if requested != "" && requested != "auto" {
		iface, err := net.InterfaceByName(requested)
		if err != nil {
			return LocalInterface{}, fmt.Errorf("find interface %s: %w", requested, err)
		}
		ip, err := interfaceIPv4(iface)
		if err != nil {
			return LocalInterface{}, err
		}
		return LocalInterface{Interface: *iface, IPv4: ip}, nil
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return LocalInterface{}, err
	}
	sort.SliceStable(interfaces, func(i, j int) bool {
		return interfaceScore(interfaces[i]) > interfaceScore(interfaces[j])
	})
	var rejected []string
	for index := range interfaces {
		iface := &interfaces[index]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		ip, ipErr := interfaceIPv4(iface)
		if ipErr == nil {
			return LocalInterface{Interface: *iface, IPv4: ip}, nil
		}
		rejected = append(rejected, iface.Name)
	}
	return LocalInterface{}, fmt.Errorf("no active multicast-capable LAN interface with IPv4 was found (checked %s)", strings.Join(rejected, ", "))
}

func interfaceIPv4(iface *net.Interface) (net.IP, error) {
	addresses, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		var ip net.IP
		switch value := address.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		}
		if ip == nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip.To4(), nil
	}
	return nil, fmt.Errorf("interface %s has no usable IPv4 address", iface.Name)
}

func interfaceScore(iface net.Interface) int {
	score := 0
	switch iface.Name {
	case "en0":
		score += 100
	case "en1":
		score += 90
	}
	if strings.HasPrefix(iface.Name, "en") {
		score += 50
	}
	if iface.Flags&net.FlagBroadcast != 0 {
		score += 10
	}
	return score
}
