package orch

import (
	"time"

	"github.com/google/uuid"
	"github.com/docxology/GuildNet/internal/localdb"
)

// AppendAudit writes an audit event to the local DB for traceability.
// This does not store sensitive data; diffs should be redacted upstream.
func AppendAudit(db *localdb.DB, actor, action, entityType, entityID, diffJSON string) {
	if db == nil {
		return
	}
	m := map[string]any{
		"id":         uuid.NewString(),
		"actor":      actor,
		"action":     action,
		"entityType": entityType,
		"entityId":   entityID,
		"diffJSON":   diffJSON,
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
	}
	_ = db.Put("audit", m["id"].(string), m)
}
