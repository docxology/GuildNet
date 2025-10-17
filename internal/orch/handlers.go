package orch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/docxology/GuildNet/internal/headscale"
	"github.com/docxology/GuildNet/internal/jobs"
	"github.com/docxology/GuildNet/internal/localdb"
	"github.com/docxology/GuildNet/internal/secrets"
)

// Deps carries minimal dependencies for orchestration handlers.
type Deps struct {
	DB      *localdb.DB
	Secrets *secrets.Manager
}

// HandlerFor returns a jobs handler function for a given kind.
func HandlerFor(kind string, deps Deps) func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
	switch kind {
	case "headscale.create":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			mgr := headscale.New(deps.DB, deps.Secrets)
			_ = mgr.Create(ctx, id, logf)
			j.Progress = 1
		}
	case "headscale.start":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			mgr := headscale.New(deps.DB, deps.Secrets)
			_ = mgr.Start(ctx, id, logf)
			j.Progress = 1
		}
	case "headscale.stop":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			mgr := headscale.New(deps.DB, deps.Secrets)
			_ = mgr.Stop(ctx, id, logf)
			j.Progress = 1
		}
	case "headscale.destroy":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			mgr := headscale.New(deps.DB, deps.Secrets)
			_ = mgr.Destroy(ctx, id, logf)
			j.Progress = 1
		}
	case "cluster.create":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			name := fmt.Sprint(spec["name"])
			if id == "" {
				return
			}
			logf("create", "registering cluster", map[string]any{"id": id, "name": name})
			if deps.DB != nil {
				var rec map[string]any
				if err := deps.DB.Get("clusters", id, &rec); err == nil {
					rec["state"] = "ready"
					rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
					_ = deps.DB.Put("clusters", id, rec)
				}
			}
			j.Progress = 1
		}
	case "cluster.scale", "cluster.upgrade":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			action := "scale"
			if kind == "cluster.upgrade" {
				action = "upgrade"
			}
			logf("op", action+" cluster", map[string]any{"id": id})
			if deps.DB != nil {
				var rec map[string]any
				if err := deps.DB.Get("clusters", id, &rec); err == nil {
					rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
					_ = deps.DB.Put("clusters", id, rec)
				}
			}
			j.Progress = 1
		}
	case "cluster.destroy":
		return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
			var spec map[string]any
			_ = json.Unmarshal([]byte(j.SpecJSON), &spec)
			id := fmt.Sprint(spec["id"])
			if id == "" {
				return
			}
			logf("op", "destroy cluster", map[string]any{"id": id})
			if deps.DB != nil {
				_ = deps.DB.Delete("clusters", id)
			}
			j.Progress = 1
		}
	}
	// default no-op handler
	return func(ctx context.Context, j *jobs.Record, logf func(step, msg string, kv map[string]any)) {
		logf("noop", "unhandled job kind", map[string]any{"kind": kind})
		j.Progress = 1
	}
}
