package settings

import (
	"strings"

	"github.com/your/module/internal/localdb"
)

// Tailscale holds tsnet control-plane settings managed at runtime.
type Tailscale struct {
	LoginServer string `json:"login_server"`
	PreauthKey  string `json:"preauth_key"`
	Hostname    string `json:"hostname"`
}

// Database holds DB connection settings.
type Database struct {
	Addr string `json:"addr"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

// Manager wraps localdb for typed settings.
type Manager struct{ DB *localdb.DB }

const (
	bucket = "settings"
	keyTS  = "tailscale"
	keyDB  = "database"
)

func EnsureBucket(db *localdb.DB) error { return db.EnsureBuckets(bucket) }

func (m Manager) GetTailscale(out *Tailscale) error {
	var tmp map[string]any
	if err := m.DB.Get(bucket, keyTS, &tmp); err != nil {
		*out = Tailscale{}
		return nil
	}
	out.LoginServer = strings.TrimSpace(asString(tmp["login_server"]))
	out.PreauthKey = strings.TrimSpace(asString(tmp["preauth_key"]))
	out.Hostname = strings.TrimSpace(asString(tmp["hostname"]))
	return nil
}

func (m Manager) PutTailscale(ts Tailscale) error {
	rec := map[string]any{
		"login_server": strings.TrimSpace(ts.LoginServer),
		"preauth_key":  strings.TrimSpace(ts.PreauthKey),
		"hostname":     strings.TrimSpace(ts.Hostname),
	}
	return m.DB.Put(bucket, keyTS, rec)
}

func (m Manager) GetDatabase(out *Database) error {
	var tmp map[string]any
	if err := m.DB.Get(bucket, keyDB, &tmp); err != nil {
		*out = Database{}
		return nil
	}
	out.Addr = strings.TrimSpace(asString(tmp["addr"]))
	out.User = strings.TrimSpace(asString(tmp["user"]))
	out.Pass = strings.TrimSpace(asString(tmp["pass"]))
	return nil
}

func (m Manager) PutDatabase(db Database) error {
	rec := map[string]any{
		"addr": strings.TrimSpace(db.Addr),
		"user": strings.TrimSpace(db.User),
		"pass": strings.TrimSpace(db.Pass),
	}
	return m.DB.Put(bucket, keyDB, rec)
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
