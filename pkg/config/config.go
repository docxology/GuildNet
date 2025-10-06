package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	LoginServer    string   `json:"login_server"`
	AuthKey        string   `json:"auth_key"`
	Hostname       string   `json:"hostname"`
	ListenLocal    string   `json:"listen_local"`
	DialTimeoutMS  int      `json:"dial_timeout_ms"`
	Allowlist      []string `json:"allowlist"`
	Name           string   `json:"name,omitempty"`
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil { return h }
	return "."
}

func baseDir() string { return filepath.Join(homeDir(), ".guildnet") }

func StateDir() string { return filepath.Join(baseDir(), "state") }

func ConfigPath() string { return filepath.Join(baseDir(), "config.json") }

func Load() (*Config, error) {
	b, err := os.ReadFile(ConfigPath())
	if err != nil { return nil, err }
	var c Config
	if err := json.Unmarshal(b, &c); err != nil { return nil, err }
	return &c, nil
}

func Save(c *Config) error {
	if err := os.MkdirAll(baseDir(), 0o700); err != nil { return err }
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(ConfigPath(), b, 0o600)
}

func (c *Config) Validate() error {
	devNoTS := os.Getenv("DEV_NO_TSNET") == "1"
	if !devNoTS {
		if !strings.HasPrefix(c.LoginServer, "http://") && !strings.HasPrefix(c.LoginServer, "https://") {
			return errors.New("login_server must be a URL")
		}
		if c.AuthKey == "" { return errors.New("auth_key required") }
		if c.Hostname == "" { return errors.New("hostname required") }
	}
	if c.ListenLocal == "" { return errors.New("listen_local required") }
	if c.DialTimeoutMS <= 0 || c.DialTimeoutMS > 60000 { return fmt.Errorf("dial_timeout_ms out of range: %d", c.DialTimeoutMS) }
	// validate allowlist entries
	for _, it := range c.Allowlist {
		it = strings.TrimSpace(it)
		if it == "" { continue }
		if strings.Contains(it, "/") {
			if _, _, err := net.ParseCIDR(it); err != nil { return fmt.Errorf("invalid cidr %q", it) }
			continue
		}
		if h, p, ok := strings.Cut(it, ":"); !ok || h == "" || p == "" { return fmt.Errorf("invalid allowlist item %q", it) }
	}
	return nil
}
