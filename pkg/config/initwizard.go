package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func RunInitWizard(in *os.File, out *os.File) error {
	fmt.Fprintln(out, "GuildNet Host App setup wizard")
	fmt.Fprintln(out, "Config will be stored under:", baseDir())

	read := func(prompt, def string) (string, error) {
		fmt.Fprintf(out, "%s [%s]: ", prompt, def)
		s := bufio.NewScanner(in)
		if !s.Scan() { return def, s.Err() }
		v := strings.TrimSpace(s.Text())
		if v == "" { return def, nil }
		return v, nil
	}

	login, _ := read("Login server URL (Headscale)", "https://headscale.example.com")
	auth, _ := read("Pre-auth key", "tskey-abc123")
	host, _ := read("Hostname", "host-app")
	listen, _ := read("Listen local", "127.0.0.1:8080")
	dialStr, _ := read("Dial timeout ms", "3000")
	allowStr, _ := read("Allowlist entries (comma-separated CIDRs or host:port)", "")
	name, _ := read("Profile/cluster name (optional)", "")

	al := []string{}
	for _, p := range strings.Split(allowStr, ",") {
		p = strings.TrimSpace(p)
		if p != "" { al = append(al, p) }
	}

	c := &Config{
		LoginServer:   login,
		AuthKey:       auth,
		Hostname:      host,
		ListenLocal:   listen,
		DialTimeoutMS: atoiDefault(dialStr, 3000),
		Allowlist:     al,
		Name:          name,
	}
	if err := os.MkdirAll(StateDir(), 0o700); err != nil { return err }
	if err := c.Validate(); err != nil { return err }
	return Save(c)
}

func atoiDefault(s string, def int) int {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil || n <= 0 { return def }
	return n
}
