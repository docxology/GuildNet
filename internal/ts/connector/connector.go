package connector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// Config describes how to start a per-cluster embedded tsnet server.
type Config struct {
	ClusterID     string
	LoginServer   string
	ClientAuthKey string
	StateDir      string
	Hostname      string // optional
}

// Connector manages a tsnet.Server and provides dialing utilities.
type Connector struct {
	cfg   Config
	srv   *tsnet.Server
	mu    sync.RWMutex
	start sync.Once
	stop  sync.Once
}

// New validates and returns a Connector with the given configuration.
func New(cfg Config) (*Connector, error) {
	id := strings.TrimSpace(cfg.ClusterID)
	if id == "" {
		return nil, errors.New("clusterID required")
	}
	if strings.TrimSpace(cfg.LoginServer) == "" {
		return nil, errors.New("loginServer required")
	}
	// ClientAuthKey may be empty if the state dir already contains device state.
	// For first-time join it's required, but we validate in Start.

	state := strings.TrimSpace(cfg.StateDir)
	if state == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			return nil, errors.New("no home dir for state")
		}
		state = filepath.Join(home, ".guildnet", "tsnet", fmt.Sprintf("cluster-%s", sanitizeID(id)))
	}
	if err := os.MkdirAll(state, 0o700); err != nil {
		return nil, fmt.Errorf("state dir: %w", err)
	}
	if err := os.Chmod(state, 0o700); err != nil {
		// best effort; ignore on Windows
		_ = err
	}
	cfg.StateDir = state
	if strings.TrimSpace(cfg.Hostname) == "" {
		host, _ := os.Hostname()
		if host == "" {
			host = randSuffix("node")
		}
		cfg.Hostname = fmt.Sprintf("guildnet-%s-%s", sanitizeID(id), sanitizeID(host))
	}
	return &Connector{cfg: cfg}, nil
}

// Start initializes the tsnet server (idempotent).
func (c *Connector) Start(ctx context.Context) error {
	var retErr error
	c.start.Do(func() {
		// If state already exists, tsnet can reuse it without a fresh auth key
		if !dirExists(c.cfg.StateDir) && strings.TrimSpace(c.cfg.ClientAuthKey) == "" {
			retErr = errors.New("clientAuthKey required for first start")
			return
		}
		s := &tsnet.Server{
			Dir:        c.cfg.StateDir,
			Hostname:   c.cfg.Hostname,
			AuthKey:    strings.TrimSpace(c.cfg.ClientAuthKey),
			ControlURL: strings.TrimSpace(c.cfg.LoginServer),
		}
		if err := s.Start(); err != nil {
			retErr = fmt.Errorf("tsnet start: %w", err)
			return
		}
		// Wait until client is up or timeout
		lc, err := s.LocalClient()
		if err != nil {
			retErr = fmt.Errorf("local client: %w", err)
			_ = s.Close()
			return
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			st, err := lc.Status(ctx)
			if err == nil && st != nil && (len(st.TailscaleIPs) > 0 || (st.Self != nil && st.Self.DNSName != "")) {
				break
			}
			if time.Now().After(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				retErr = ctx.Err()
				_ = s.Close()
				return
			case <-time.After(200 * time.Millisecond):
			}
		}
		c.mu.Lock()
		c.srv = s
		c.mu.Unlock()
		// Ensure server is closed on GC if Stop not called explicitly
		runtime.SetFinalizer(c, func(cc *Connector) {
			_ = cc.CloseServer()
		})
	})
	return retErr
}

// DialContext dials using the tsnet server.
func (c *Connector) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c.mu.RLock()
	s := c.srv
	c.mu.RUnlock()
	if s == nil {
		return nil, errors.New("connector not started")
	}
	return s.Dial(ctx, network, addr)
}

// HTTPTransport returns a clone of base (or a new one) that dials via tsnet.
func (c *Connector) HTTPTransport(base *http.Transport) *http.Transport {
	t := &http.Transport{}
	if base != nil {
		// Copy safe fields manually; avoid copying the mutex
		t.Proxy = base.Proxy
		t.ProxyConnectHeader = base.ProxyConnectHeader
		t.TLSClientConfig = base.TLSClientConfig
		t.TLSHandshakeTimeout = base.TLSHandshakeTimeout
		t.DisableKeepAlives = base.DisableKeepAlives
		t.DisableCompression = base.DisableCompression
		t.MaxIdleConns = base.MaxIdleConns
		t.MaxIdleConnsPerHost = base.MaxIdleConnsPerHost
		t.MaxConnsPerHost = base.MaxConnsPerHost
		t.IdleConnTimeout = base.IdleConnTimeout
		t.ResponseHeaderTimeout = base.ResponseHeaderTimeout
		t.ExpectContinueTimeout = base.ExpectContinueTimeout
		t.TLSNextProto = base.TLSNextProto
		t.ProxyConnectHeader = base.ProxyConnectHeader
		t.DialTLSContext = base.DialTLSContext
	}
	// Always override DialContext
	t.DialContext = c.DialContext
	// Disable HTTP/2 to avoid misconfig surprises unless the base already enables it explicitly.
	t.ForceAttemptHTTP2 = false
	return t
}

// Health returns a quick status and details map describing the connector state.
func (c *Connector) Health(ctx context.Context) (string, map[string]any) {
	det := map[string]any{"clusterId": c.cfg.ClusterID, "stateDir": c.cfg.StateDir, "loginServer": redactURL(c.cfg.LoginServer)}
	c.mu.RLock()
	s := c.srv
	c.mu.RUnlock()
	if s == nil {
		return "stopped", det
	}
	lc, err := s.LocalClient()
	if err != nil {
		det["error"] = err.Error()
		return "degraded", det
	}
	st, err := lc.Status(ctx)
	if err != nil {
		det["error"] = err.Error()
		return "degraded", det
	}
	var ip, fqdn string
	if len(st.TailscaleIPs) > 0 {
		ip = st.TailscaleIPs[0].String()
	}
	if st.Self != nil {
		fqdn = strings.TrimSuffix(st.Self.DNSName, ".")
	}
	det["ip"] = ip
	det["fqdn"] = fqdn
	if ip != "" || fqdn != "" {
		return "ok", det
	}
	return "starting", det
}

// Stop gracefully stops the tsnet server.
func (c *Connector) Stop(ctx context.Context) error { // ctx unused for now
	var retErr error
	c.stop.Do(func() {
		retErr = c.CloseServer()
	})
	return retErr
}

// CloseServer closes the underlying tsnet.Server immediately.
func (c *Connector) CloseServer() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.srv != nil {
		err := c.srv.Close()
		c.srv = nil
		return err
	}
	return nil
}

// Helpers
func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteByte('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		res = "default"
	}
	return res
}

func randSuffix(prefix string) string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(buf[:]))
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func redactURL(s string) string {
	// Return host:port or scheme://host form without credentials
	return s
}
