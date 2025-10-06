package ts

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"tailscale.com/tsnet"
)

// Options holds tsnet server init configuration.
type Options struct {
	StateDir  string
	Hostname  string
	LoginURL  string
	AuthKey   string
}

// StartServer initializes and starts a tsnet.Server.
func StartServer(ctx context.Context, opts Options) (*tsnet.Server, error) {
	s := &tsnet.Server{
		Dir:       opts.StateDir,
		Hostname:  opts.Hostname,
		AuthKey:   opts.AuthKey,
	ControlURL:  opts.LoginURL,
	}
	if err := s.Start(); err != nil {
		return nil, fmt.Errorf("tsnet start: %w", err)
	}
	return s, nil
}

// Listen creates a listener on the tsnet server.
func Listen(ctx context.Context, s *tsnet.Server, network, addr string) (net.Listener, error) {
	// tsnet.Listen does not require context in current API
	_ = ctx
	return s.Listen(network, addr)
}

// DialContext dials using the tsnet server's netstack.
func DialContext(ctx context.Context, s *tsnet.Server, network, addr string) (net.Conn, error) {
	return s.Dial(ctx, network, addr)
}

// Info retrieves the current node's IP and MagicDNS name.
type InfoResult struct {
	IP   string
	FQDN string
}

func Info(ctx context.Context, s *tsnet.Server) (*InfoResult, error) {
	lc, err := s.LocalClient()
	if err != nil { return nil, err }
	// Wait until we have an IP or timeout
	deadline := time.Now().Add(30 * time.Second)
	var ipStr, fqdn string
	for {
		st, err := lc.Status(ctx)
		if err == nil && st != nil {
			if len(st.TailscaleIPs) > 0 {
				ipStr = st.TailscaleIPs[0].String()
			}
			if st.Self != nil {
				fqdn = strings.TrimSuffix(st.Self.DNSName, ".")
			}
			if ipStr != "" || fqdn != "" {
				break
			}
		}
		if time.Now().After(deadline) { break }
		time.Sleep(200 * time.Millisecond)
	}
	return &InfoResult{IP: ipStr, FQDN: fqdn}, nil
}
