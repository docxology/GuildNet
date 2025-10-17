package tests

import (
	"github.com/docxology/GuildNet/internal/proxy"
	"testing"
)

func TestAllowlist(t *testing.T) {
	al, err := proxy.NewAllowlist([]string{"10.0.0.0/8", "db.local:5432"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host string
		port int
		ok   bool
	}{
		{"10.1.2.3", 80, true},
		{"10.255.255.255", 1, true},
		{"11.0.0.1", 80, false},
		{"db.local", 5432, true},
		{"db.local", 5433, false},
	}
	for _, c := range cases {
		if got := al.Allowed(c.host, c.port); got != c.ok {
			t.Fatalf("allowed(%s,%d)=%v want %v", c.host, c.port, got, c.ok)
		}
	}
}
