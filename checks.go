package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// getServers reads and validates the forwardings file at path.
func getServers(path string) ([]serverInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %w", path, err)
	}
	var allServers []serverInfo
	if err := json.Unmarshal(data, &allServers); err != nil {
		return nil, fmt.Errorf("cannot parse %q: %w", path, err)
	}
	if len(allServers) == 0 {
		return nil, fmt.Errorf("the configuration file %q does not contain any server definitions", path)
	}

	hosts := make(map[string]struct{})
	listenAddrs := make(map[string]string) // normalized listen address -> "host %q service %q"

	for idx, server := range allServers {
		s := server
		s.Host = strings.TrimSpace(server.Host)

		if s.Host == "" {
			return nil, fmt.Errorf("file %q: server #%d has an empty %q", path, idx+1, "Host")
		}
		// The host is passed to ssh as the destination argument. A value
		// starting with '-' would be parsed by ssh as an option, and an
		// option such as -oProxyCommand runs an arbitrary command.
		if strings.HasPrefix(s.Host, "-") {
			return nil, fmt.Errorf("file %q: host %q is not valid, it must not start with %q", path, s.Host, "-")
		}
		if strings.ContainsAny(s.Host, " \t\r\n") {
			return nil, fmt.Errorf("file %q: host %q must not contain whitespace", path, s.Host)
		}
		hostKey := strings.ToLower(s.Host)
		if _, ok := hosts[hostKey]; ok {
			return nil, fmt.Errorf("file %q: host %q is declared multiple times", path, s.Host)
		}
		hosts[hostKey] = struct{}{}

		if len(s.Forward) == 0 {
			return nil, fmt.Errorf("file %q: host %q has no forwardings", path, s.Host)
		}

		services := make(map[string]struct{})
		for jdx, fw := range s.Forward {
			fw.Service = strings.TrimSpace(fw.Service)
			fw.ListenAddr = strings.TrimSpace(fw.ListenAddr)
			fw.RemoteAddr = strings.TrimSpace(fw.RemoteAddr)

			if fw.Service == "" {
				return nil, fmt.Errorf("file %q: host %q, forwarding #%d has an empty %q", path, s.Host, jdx+1, "Service")
			}
			svcKey := strings.ToLower(fw.Service)
			if _, ok := services[svcKey]; ok {
				return nil, fmt.Errorf("file %q: service %q is declared multiple times under host %q", path, fw.Service, s.Host)
			}
			services[svcKey] = struct{}{}

			listen, err := validateAddr(fw.ListenAddr, true)
			if err != nil {
				return nil, fmt.Errorf("file %q: host %q, service %q: %w", path, s.Host, fw.Service, err)
			}
			if _, err := validateAddr(fw.RemoteAddr, false); err != nil {
				return nil, fmt.Errorf("file %q: host %q, service %q: %w", path, s.Host, fw.Service, err)
			}
			if prev, ok := listenAddrs[listen]; ok {
				return nil, fmt.Errorf("file %q: %q is already forwarded by %s", path, listen, prev)
			}
			listenAddrs[listen] = fmt.Sprintf("host %q service %q", s.Host, fw.Service)

			s.Forward[jdx] = fw
		}
		allServers[idx] = s
	}
	return allServers, nil
}

// validateAddr checks a "host:port" string and returns it normalized.
// listen selects the message for a wildcard address: binding to 0.0.0.0 or ::
// exposes the forwarded service on every interface, which defeats the purpose
// of the program, so it is always refused.
func validateAddr(addr string, listen bool) (string, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid address %q, expected host:port", addr)
	}
	if host == "" {
		return "", fmt.Errorf("invalid address %q: missing host", addr)
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return "", fmt.Errorf("invalid address %q: host contains whitespace", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid address %q: port must be a number from 1 to 65535", addr)
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		if listen {
			return "", fmt.Errorf("invalid address %q: a wildcard address exposes the service on every interface; use an address such as 127.0.0.1 instead", addr)
		}
		return "", fmt.Errorf("invalid address %q: wildcard address", addr)
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port)), nil
}
