package getip

import (
	"fmt"
	"net"
)

// ContainerIPv4s returns the container's IPv4 addresses, optionally preferring a given interface (e.g. "eth0").
func ContainerIPv4s(preferIface string) ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var preferred, others []net.IP

	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP.To4()
			case *net.IPAddr:
				ip = v.IP.To4()
			}
			if ip == nil {
				continue // skip non-IPv4
			}
			if preferIface != "" && iface.Name == preferIface {
				preferred = append(preferred, ip)
			} else {
				others = append(others, ip)
			}
		}
	}

	ips := append(preferred, others...)
	if len(ips) == 0 {
		return nil, fmt.Errorf("no non-loopback IPv4 addresses found")
	}
	return ips, nil
}

// PrimaryIPv4 returns the first IPv4 from ContainerIPv4s (useful as "the container IP").
func PrimaryIPv4(preferIface string) (net.IP, error) {
	ips, err := ContainerIPv4s(preferIface)
	if err != nil {
		return nil, err
	}
	return ips[0], nil
}
