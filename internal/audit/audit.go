package audit

import (
	"time"

	"github.com/google/uuid"
	"github.com/your/module/internal/localdb"
)

// Append writes an audit event to the local DB for traceability.
// Do not include sensitive values; redact upstream if needed.
func Append(db *localdb.DB, actor, action, entityType, entityID, diffJSON string) {
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
