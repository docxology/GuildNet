package proxy

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type entry interface {
	Allowed(host string, port int) bool
}

type Allowlist struct {
	entries []entry
}

func NewAllowlist(items []string) (*Allowlist, error) {
	al := &Allowlist{}
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if strings.Contains(it, "/") {
			_, ipnet, err := net.ParseCIDR(it)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", it, err)
			}
			al.entries = append(al.entries, cidrEntry{net: ipnet})
			continue
		}
		// host:port
		h, p, ok := strings.Cut(it, ":")
		if !ok {
			return nil, fmt.Errorf("invalid allowlist item %q (want host:port or CIDR)", it)
		}
		port, err := strconv.Atoi(p)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid port in %q", it)
		}
		al.entries = append(al.entries, hostPortEntry{host: h, port: port})
	}
	return al, nil
}

func (a *Allowlist) IsEmpty() bool { return len(a.entries) == 0 }

func (a *Allowlist) Allowed(host string, port int) bool {
	ip := net.ParseIP(host)
	for _, e := range a.entries {
		if ce, ok := e.(cidrEntry); ok {
			if ip != nil && ce.net.Contains(ip) {
				return true
			}
		}
		if e.Allowed(host, port) {
			return true
		}
	}
	return false
}

func (a *Allowlist) AllowedAddr(addr string) bool {
	h, p, ok := strings.Cut(addr, ":")
	if !ok {
		return false
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return false
	}
	return a.Allowed(h, port)
}

type cidrEntry struct{ net *net.IPNet }

func (c cidrEntry) Allowed(host string, port int) bool {
	ip := net.ParseIP(host)
	return ip != nil && c.net.Contains(ip)
}

type hostPortEntry struct {
	host string
	port int
}

func (h hostPortEntry) Allowed(host string, port int) bool {
	return strings.EqualFold(host, h.host) && port == h.port
}
