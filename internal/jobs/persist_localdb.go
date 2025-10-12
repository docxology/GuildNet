package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/your/module/internal/localdb"
)

// LocalPersist implements Persist on top of localdb.DB
// Buckets used: jobs, joblogs

type LocalPersist struct{ DB *localdb.DB }

func (p LocalPersist) SaveJob(rec Record) error {
	if p.DB == nil {
		return nil
	}
	m := map[string]any{
		"id": rec.ID, "kind": rec.Kind, "specJSON": rec.SpecJSON,
		"status": rec.Status, "progress": rec.Progress,
		"created": rec.Created.Format(time.RFC3339Nano),
		"updated": rec.Updated.Format(time.RFC3339Nano),
		"result":  string(rec.Result), "error": rec.Error,
	}
	return p.DB.Put("jobs", rec.ID, m)
}

func (p LocalPersist) AppendLog(jobID string, e LogEvent) error {
	if p.DB == nil {
		return nil
	}
	b, _ := json.Marshal(e)
	b = append(b, '\n')
	return p.DB.AppendLog("joblogs", jobID, b)
}

func (p LocalPersist) ListJobs() ([]Record, error) {
	if p.DB == nil {
		return nil, nil
	}
	var arr []map[string]any
	if err := p.DB.List("jobs", &arr); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(arr))
	for _, m := range arr {
		var rec Record
		if v, _ := m["id"].(string); v != "" {
			rec.ID = v
		}
		if v, _ := m["kind"].(string); v != "" {
			rec.Kind = v
		}
		if v, _ := m["specJSON"].(string); v != "" {
			rec.SpecJSON = v
		}
		if v, _ := m["status"].(string); v != "" {
			rec.Status = Status(v)
		}
		if v, ok := m["progress"].(float64); ok {
			rec.Progress = v
		}
		if v, _ := m["created"].(string); v != "" {
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				rec.Created = t
			}
		}
		if v, _ := m["updated"].(string); v != "" {
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				rec.Updated = t
			}
		}
		if v, _ := m["result"].(string); v != "" {
			rec.Result = []byte(v)
		}
		if v, _ := m["error"].(string); v != "" {
			rec.Error = v
		}
		if rec.ID == "" {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

func (p LocalPersist) GetJob(id string) (*Record, error) {
	if p.DB == nil {
		return nil, nil
	}
	var m map[string]any
	if err := p.DB.Get("jobs", id, &m); err != nil {
		return nil, err
	}
	arr, err := p.ListJobs()
	if err != nil {
		return nil, err
	}
	for _, r := range arr {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
