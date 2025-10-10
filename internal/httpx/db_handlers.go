package httpx

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/your/module/internal/db"
	"github.com/your/module/internal/metrics"
	"github.com/your/module/internal/model"
)

// DBAPI bundles dependencies needed by database handlers.
type DBAPI struct {
	Manager DBManager
	// For MVP we assume single org until auth/tenancy implemented. Stub OrgID.
	OrgID string
	RBAC  *RBACStore
}

// Register attaches handlers to mux.
func (a *DBAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/db", a.handleDatabases)
	mux.HandleFunc("/api/db/", a.handleDatabaseSubroutes)
	// SSE changefeed
	mux.HandleFunc("/sse/db/", a.handleChangefeed)
	// DB connectivity health
	mux.HandleFunc("/api/db/health", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		resp := map[string]any{"status": status}
		if a.Manager == nil {
			// Best-effort lazy connect: if DB becomes reachable after server start, initialize manager now.
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			addr := db.AutoDiscoverAddr()
			if mgr, err := db.Connect(ctx); err == nil {
				a.Manager = mgr
			} else {
				status = "unavailable"
				resp["status"] = status
				resp["addr"] = addr
				resp["error"] = err.Error()
				log.Printf("db health lazy connect failed: addr=%s err=%v", addr, err)
			}
		}
		JSON(w, http.StatusOK, resp)
	})
}

func (a *DBAPI) handleDatabases(w http.ResponseWriter, r *http.Request) {
	principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
	switch r.Method {
	case http.MethodGet:
		// List databases for org
		if a.Manager == nil {
			JSON(w, http.StatusOK, []model.DatabaseInstance{})
			return
		}
		dbs, err := a.Manager.ListDatabases(r.Context(), a.OrgID)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "list failed", "list_failed", err.Error())
			return
		}
		JSON(w, http.StatusOK, dbs)
	case http.MethodPost:
		if !Allow(a.RBAC.RoleFor(principal, "", a.OrgID), "db.create") {
			JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
			return
		}
		// Parse create payload (name optional)
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(b, &req)
		if strings.TrimSpace(req.ID) == "" {
			JSONError(w, http.StatusBadRequest, "missing id", "invalid_id")
			return
		}
		if strings.TrimSpace(req.Name) == "" {
			req.Name = req.ID
		}
		if a.Manager == nil {
			JSONError(w, http.StatusServiceUnavailable, "database unavailable", "db_unavailable")
			return
		}
		inst, err := a.Manager.CreateDatabase(r.Context(), a.OrgID, req.ID, req.Name, req.Description)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "database create failed", "db_create_failed", err.Error())
			return
		}
		// Auto-grant maintainer on the new DB to the creator principal (MVP convenience)
		if a.RBAC != nil && strings.TrimSpace(principal) != "" {
			a.RBAC.Grant(model.PermissionBinding{Principal: principal, Scope: "db:" + req.ID, Role: model.RoleMaintainer, CreatedAt: model.NowISO()})
		}
		JSON(w, http.StatusCreated, inst)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// /api/db/:dbId/... subroutes
func (a *DBAPI) handleDatabaseSubroutes(w http.ResponseWriter, r *http.Request) {
	// path after /api/db/
	tail := strings.TrimPrefix(r.URL.Path, "/api/db/")
	if tail == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	parts := strings.Split(tail, "/")
	if len(parts) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	dbID := parts[0]
	if dbID == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			if a.Manager == nil {
				JSONError(w, http.StatusServiceUnavailable, "database unavailable", "db_unavailable")
				return
			}
			info, err := a.Manager.GetDatabase(r.Context(), a.OrgID, dbID)
			if err != nil {
				JSONError(w, http.StatusNotFound, "database not found", "not_found")
				return
			}
			JSON(w, http.StatusOK, info)
			return
		}
		if r.Method == http.MethodPatch {
			// For MVP, PATCH not implemented for multi-DB (future: update _info)
			JSONError(w, http.StatusBadRequest, "update not implemented", "not_implemented")
			return
		}
		if r.Method == http.MethodDelete {
			if a.Manager == nil {
				JSONError(w, http.StatusServiceUnavailable, "database unavailable", "db_unavailable")
				return
			}
			if err := a.Manager.DeleteDatabase(r.Context(), a.OrgID, dbID); err != nil {
				JSONError(w, http.StatusInternalServerError, "delete failed", "delete_failed", err.Error())
				return
			}
			JSON(w, http.StatusOK, map[string]any{"deleted": dbID})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// route /tables
	if len(parts) >= 2 && parts[1] == "tables" {
		a.handleTables(w, r, dbID, parts[2:])
		return
	}
	// route /audit
	if len(parts) >= 2 && parts[1] == "audit" {
		principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
		_ = principal // future: filter by role, actor
		events, err := a.Manager.ListAudit(r.Context(), a.OrgID, dbID, 200)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "audit list failed", "audit_failed", err.Error())
			return
		}
		JSON(w, http.StatusOK, events)
		return
	}
	// permissions list/create (MVP in-memory)
	if len(parts) >= 2 && parts[1] == "permissions" {
		if r.Method == http.MethodGet {
			a.RBAC.mu.RLock()
			defer a.RBAC.mu.RUnlock()
			var out []model.PermissionBinding
			for _, arr := range a.RBAC.bindings {
				out = append(out, arr...)
			}
			JSON(w, http.StatusOK, out)
			return
		}
		if r.Method == http.MethodPost {
			principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
			if !Allow(a.roleFor(principal, "", dbID), "table.schema") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			var req model.PermissionBinding
			if err := json.Unmarshal(b, &req); err != nil || strings.TrimSpace(req.Principal) == "" || strings.TrimSpace(req.Scope) == "" {
				JSONError(w, http.StatusBadRequest, "invalid permission", "invalid_perm")
				return
			}
			req.CreatedAt = model.NowISO()
			a.RBAC.Grant(req)
			JSON(w, http.StatusCreated, req)
			return
		}
		if r.Method == http.MethodDelete {
			principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
			if !Allow(a.roleFor(principal, "", dbID), "table.schema") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			scope := r.URL.Query().Get("scope")
			who := r.URL.Query().Get("principal")
			if strings.TrimSpace(scope) == "" || strings.TrimSpace(who) == "" {
				JSONError(w, http.StatusBadRequest, "missing scope/principal", "invalid_perm")
				return
			}
			a.RBAC.Revoke(scope, who)
			JSON(w, http.StatusOK, map[string]any{"revoked": true})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// roleFor returns the role resolved by table->db->org fallback.
func (a *DBAPI) roleFor(principal, tableID, dbID string) model.Role {
	role := a.RBAC.RoleFor(principal, tableID, dbID)
	if role == "" {
		role = a.RBAC.RoleFor(principal, "", a.OrgID)
	}
	return role
}

func (a *DBAPI) handleTables(w http.ResponseWriter, r *http.Request, dbID string, rest []string) {
	if a.Manager == nil {
		JSONError(w, http.StatusServiceUnavailable, "database unavailable", "db_unavailable")
		return
	}
	principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
	if len(rest) == 0 { // /api/db/:dbId/tables
		switch r.Method {
		case http.MethodGet:
			tbls, err := a.Manager.GetTables(r.Context(), a.OrgID, dbID)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "list tables failed", "list_failed", err.Error())
				return
			}
			if tbls == nil {
				tbls = []model.Table{}
			}
			JSON(w, http.StatusOK, tbls)
		case http.MethodPost:
			if !Allow(a.roleFor(principal, "", dbID), "table.create") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			var req struct {
				Name       string            `json:"name"`
				Schema     []model.ColumnDef `json:"schema"`
				PrimaryKey string            `json:"primary_key"`
			}
			if err := json.Unmarshal(b, &req); err != nil || strings.TrimSpace(req.Name) == "" {
				JSONError(w, http.StatusBadRequest, "invalid table spec", "invalid_spec")
				return
			}
			tbl := model.Table{ID: req.Name, Name: req.Name, PrimaryKey: req.PrimaryKey, Schema: req.Schema}
			if err := a.Manager.CreateTable(r.Context(), a.OrgID, dbID, tbl); err != nil {
				JSONError(w, http.StatusInternalServerError, "table create failed", "create_failed", err.Error())
				return
			}
			JSON(w, http.StatusCreated, tbl)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	tableName := rest[0]
	if len(rest) == 1 { // metadata GET/PATCH/DELETE
		if r.Method == http.MethodGet {
			// naive lookup
			tbls, _ := a.Manager.GetTables(r.Context(), a.OrgID, dbID)
			for _, t := range tbls {
				if t.Name == tableName {
					JSON(w, http.StatusOK, t)
					return
				}
			}
			JSONError(w, http.StatusNotFound, "table not found", "not_found")
			return
		}
		if r.Method == http.MethodPatch {
			principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
			if !Allow(a.roleFor(principal, tableName, dbID), "table.schema") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			var req struct {
				Schema     []model.ColumnDef `json:"schema"`
				PrimaryKey string            `json:"primary_key"`
			}
			if err := json.Unmarshal(b, &req); err != nil || len(req.Schema) == 0 {
				JSONError(w, http.StatusBadRequest, "invalid schema", "invalid_schema")
				return
			}
			if err := a.Manager.UpdateTableSchema(r.Context(), a.OrgID, dbID, tableName, req.Schema, req.PrimaryKey); err != nil {
				JSONError(w, http.StatusInternalServerError, "schema update failed", "schema_failed", err.Error())
				return
			}
			JSON(w, http.StatusOK, map[string]any{"updated": tableName})
			return
		}
		if r.Method == http.MethodDelete {
			if !Allow(a.roleFor(principal, tableName, dbID), "table.delete") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			JSONError(w, http.StatusBadRequest, "delete not implemented", "not_implemented")
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// /rows operations
	if len(rest) >= 2 && rest[1] == "rows" {
		a.handleRows(w, r, dbID, tableName, rest[2:])
		return
	}
	// /views placeholder
	if len(rest) >= 2 && rest[1] == "views" {
		JSON(w, http.StatusOK, []model.View{})
		return
	}
	// /import
	if len(rest) >= 2 && rest[1] == "import" {
		principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
		if !Allow(a.roleFor(principal, tableName, dbID), "row.write") {
			JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		// obtain schema for validation
		schema := []model.ColumnDef{}
		if tbls, _ := a.Manager.GetTables(r.Context(), a.OrgID, dbID); true {
			for _, t := range tbls {
				if t.Name == tableName {
					schema = t.Schema
					break
				}
			}
		}
		ct := r.Header.Get("Content-Type")
		dryRun := r.URL.Query().Get("dry_run") == "1"
		data, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var rows []map[string]any
		var preview []model.ImportPreviewRow
		// optional client-provided mapping from source->target column names
		mapping := map[string]string{}
		normalize := func(in map[string]any) map[string]any {
			if len(mapping) == 0 {
				return in
			}
			out := make(map[string]any, len(in))
			for k, v := range in {
				if to, ok := mapping[k]; ok && to != "" {
					out[to] = v
				} else {
					out[k] = v
				}
			}
			return out
		}
		if strings.Contains(ct, "text/csv") {
			cr := csv.NewReader(bytes.NewReader(data))
			records, err := cr.ReadAll()
			if err != nil {
				JSONError(w, http.StatusBadRequest, "csv parse failed", "csv_parse", err.Error())
				return
			}
			if len(records) == 0 {
				JSONError(w, http.StatusBadRequest, "empty csv", "empty_csv")
				return
			}
			head := records[0]
			for i := 1; i < len(records); i++ {
				rec := records[i]
				if len(rec) == 0 {
					continue
				}
				m := map[string]any{}
				for j, col := range head {
					if j < len(rec) {
						m[col] = rec[j]
					}
				}
				rows = append(rows, m)
			}
		} else { // json
			if len(bytes.TrimSpace(data)) == 0 {
				JSONError(w, http.StatusBadRequest, "empty body", "empty_body")
				return
			}
			if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
				var wrapper struct {
					Rows    any               `json:"rows"`
					Dry     bool              `json:"dry_run"`
					Mapping map[string]string `json:"mapping"`
				}
				if err := json.Unmarshal(data, &wrapper); err != nil {
					JSONError(w, http.StatusBadRequest, "invalid json", "bad_json", err.Error())
					return
				}
				if wrapper.Dry {
					dryRun = true
				}
				if wrapper.Mapping != nil {
					mapping = wrapper.Mapping
				}
				switch v := wrapper.Rows.(type) {
				case []any:
					for _, it := range v {
						if m, ok := it.(map[string]any); ok {
							rows = append(rows, m)
						}
					}
				case map[string]any:
					rows = append(rows, v)
				default:
					JSONError(w, http.StatusBadRequest, "rows must be object or array", "bad_rows")
					return
				}
			} else if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
				var arr []map[string]any
				if err := json.Unmarshal(data, &arr); err != nil {
					JSONError(w, http.StatusBadRequest, "invalid json array", "bad_json", err.Error())
					return
				}
				rows = arr
			} else {
				JSONError(w, http.StatusBadRequest, "unsupported body", "unsupported_body")
				return
			}
		}
		// validation & preview
		if dryRun {
			for idx, row := range rows {
				if idx >= 50 {
					break
				} // cap preview
				mapped := normalize(row)
				pr := model.ImportPreviewRow{Raw: row, Mapped: map[string]any{}}
				errs := []string{}
				for _, col := range schema {
					val, ok := mapped[col.Name]
					if !ok {
						if col.Required {
							errs = append(errs, "missing:"+col.Name)
						}
						continue
					}
					// simple type checks
					if !validateType(col.Type, val) {
						errs = append(errs, "type:"+col.Name)
					}
					pr.Mapped[col.Name] = val
				}
				pr.Errors = errs
				preview = append(preview, pr)
			}
			JSON(w, http.StatusOK, model.ImportPreviewResponse{Rows: preview, Total: len(rows), ColumnMap: mapping})
			return
		}
		// apply mapping for actual import if provided
		if len(mapping) > 0 {
			remapped := make([]map[string]any, 0, len(rows))
			for _, r0 := range rows {
				remapped = append(remapped, normalize(r0))
			}
			rows = remapped
		}
		ids, err := a.Manager.InsertRows(r.Context(), a.OrgID, dbID, tableName, rows)
		if err != nil {
			JSONError(w, http.StatusInternalServerError, "import failed", "import_failed", err.Error())
			return
		}
		metrics.IncOp(a.OrgID, tableName, "import", 0)
		JSON(w, http.StatusOK, map[string]any{"inserted": len(ids), "ids": ids})
		return
	}
	// /export
	if len(rest) >= 2 && rest[1] == "export" {
		principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
		role := a.roleFor(principal, tableName, dbID)
		if !Allow(role, "row.read") {
			JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
			return
		}
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}
		limit := 1000
		if ls := r.URL.Query().Get("limit"); ls != "" {
			if n, err := strconv.Atoi(ls); err == nil && n > 0 && n <= 10000 {
				limit = n
			}
		}
		// fetch in pages
		cursor := ""
		rowsAccum := make([]map[string]any, 0, limit)
		for len(rowsAccum) < limit {
			rows, next, err := a.Manager.QueryRows(r.Context(), a.OrgID, dbID, tableName, "id", 200, cursor, true)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "export query failed", "export_query", err.Error())
				return
			}
			// mask
			schema := []model.ColumnDef{}
			if tbls, _ := a.Manager.GetTables(r.Context(), a.OrgID, dbID); true {
				for _, t := range tbls {
					if t.Name == tableName {
						schema = t.Schema
						break
					}
				}
			}
			for _, row := range rows {
				rowsAccum = append(rowsAccum, MaskRow(role, schema, row))
				if len(rowsAccum) >= limit {
					break
				}
			}
			if next == "" || cursor == next {
				break
			}
			cursor = next
		}
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=export.csv")
			if len(rowsAccum) == 0 {
				_, _ = w.Write([]byte("\n"))
				return
			}
			head := make([]string, 0, len(rowsAccum[0]))
			for k := range rowsAccum[0] {
				head = append(head, k)
			}
			cw := csv.NewWriter(w)
			_ = cw.Write(head)
			for _, row := range rowsAccum {
				rec := make([]string, len(head))
				for i, col := range head {
					rec[i] = stringify(row[col])
				}
				_ = cw.Write(rec)
			}
			cw.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rowsAccum)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *DBAPI) handleRows(w http.ResponseWriter, r *http.Request, dbID, table string, rest []string) {
	principal := PrincipalFromRequest(r.Header.Get("X-Debug-Principal"))
	// collection path
	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			role := a.roleFor(principal, table, dbID)
			if !Allow(role, "row.read") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			rows, next, err := a.Manager.QueryRows(r.Context(), a.OrgID, dbID, table, "id", 50, r.URL.Query().Get("cursor"), true)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "query failed", "query_failed", err.Error())
				return
			}
			// schema for masking
			schema := []model.ColumnDef{}
			if tbls, _ := a.Manager.GetTables(r.Context(), a.OrgID, dbID); true {
				for _, t := range tbls {
					if t.Name == table {
						schema = t.Schema
						break
					}
				}
			}
			masked := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				masked = append(masked, MaskRow(role, schema, row))
			}
			JSON(w, http.StatusOK, model.QueryPage[map[string]any]{Items: masked, NextCursor: next})
		case http.MethodPost:
			if !Allow(a.roleFor(principal, table, dbID), "row.write") {
				JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			var payload any
			if err := json.Unmarshal(b, &payload); err != nil {
				JSONError(w, http.StatusBadRequest, "invalid json", "bad_json")
				return
			}
			var rows []map[string]any
			switch v := payload.(type) {
			case map[string]any:
				rows = []map[string]any{v}
			case []any:
				for _, item := range v {
					if m, ok := item.(map[string]any); ok {
						rows = append(rows, m)
					}
				}
			default:
				JSONError(w, http.StatusBadRequest, "unsupported payload", "bad_payload")
				return
			}
			ids, err := a.Manager.InsertRows(r.Context(), a.OrgID, dbID, table, rows)
			if err != nil {
				JSONError(w, http.StatusInternalServerError, "insert failed", "insert_failed", err.Error())
				return
			}
			metrics.IncOp(a.OrgID, table, "insert", 0)
			JSON(w, http.StatusCreated, map[string]any{"inserted": len(ids), "ids": ids})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	// item path /:rowId
	rowID := rest[0]
	if rowID == "" {
		JSONError(w, http.StatusBadRequest, "missing id", "missing_id")
		return
	}
	if r.Method == http.MethodPatch {
		if !Allow(a.roleFor(principal, table, dbID), "row.write") {
			JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var patch map[string]any
		if err := json.Unmarshal(b, &patch); err != nil {
			JSONError(w, http.StatusBadRequest, "invalid json", "bad_json")
			return
		}
		if err := a.Manager.UpdateRow(r.Context(), a.OrgID, dbID, table, rowID, patch); err != nil {
			JSONError(w, http.StatusInternalServerError, "update failed", "update_failed", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"updated": rowID})
		return
	}
	if r.Method == http.MethodDelete {
		if !Allow(a.roleFor(principal, table, dbID), "row.write") {
			JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
			return
		}
		if err := a.Manager.DeleteRow(r.Context(), a.OrgID, dbID, table, rowID); err != nil {
			JSONError(w, http.StatusInternalServerError, "delete failed", "delete_failed", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"deleted": rowID})
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleChangefeed implements SSE streaming for table changes: /sse/db/:dbId/tables/:table/changes
// Query params:
//
//	cursor=<token> (resume not yet implemented; placeholder)
//	pause=1 to start paused (buffering up to a bounded backlog)
func (a *DBAPI) handleChangefeed(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/sse/db/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[1] != "tables" || parts[3] != "changes" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	dbID := parts[0]
	table := parts[2]
	// Basic validation (dbID ignored for now since single-org stub)
	_ = dbID
	// Establish changefeed
	stream, err := a.Manager.SubscribeTable(r.Context(), a.OrgID, dbID, table)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "subscribe failed", "subscribe_failed", err.Error())
		return
	}
	metrics.ChangefeedInc()
	defer metrics.ChangefeedDec()
	flusher, ok := w.(http.Flusher)
	if !ok {
		JSONError(w, http.StatusInternalServerError, "stream unsupported", "stream_unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	enc := json.NewEncoder(w)
	paused := r.URL.Query().Get("pause") == "1"
	backlog := make([]model.ChangefeedEvent, 0, 512)
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	writeEvent := func(ev model.ChangefeedEvent) bool {
		if _, err := w.Write([]byte("data: ")); err != nil {
			return false
		}
		if err := enc.Encode(ev); err != nil {
			return false
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	// Send initial hello
	_ = writeEvent(model.ChangefeedEvent{Type: "init", TableID: table, TS: model.NowISO()})
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-stream.C:
			if !ok {
				return
			}
			if paused {
				if len(backlog) < cap(backlog) {
					backlog = append(backlog, ev)
				}
				// Update pending counter (lightweight) by sending a heartbeat style comment
			} else {
				if !writeEvent(ev) {
					return
				}
			}
		case <-heartbeat.C:
			if paused && len(backlog) > 0 {
				// send a status event with pending count
				_ = writeEvent(model.ChangefeedEvent{Type: "paused", TableID: table, Pending: len(backlog), TS: model.NowISO()})
			} else {
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		case <-r.Context().Done():
			return
		}
		// Check for client commands (poll query param via header upgrade not feasible in SSE; future: control channel)
	}
}

// Helper to start DBAPI after manager creation.
func InitAndRegisterDB(mux *http.ServeMux, mgr *db.Manager) {
	api := &DBAPI{Manager: DBManager(mgr), OrgID: "org-demo", RBAC: NewRBACStore()}
	api.RBAC.Grant(model.PermissionBinding{Principal: "user:demo", Scope: "db:org-demo", Role: model.RoleMaintainer, CreatedAt: model.NowISO()})
	api.Register(mux)
	log.Printf("db api registered (org=%s)", api.OrgID)
}

// Helper simple type validation for import dry-run.
func validateType(t model.ColumnType, v any) bool {
	switch t {
	case model.ColString:
		_, ok := v.(string)
		return ok
	case model.ColNumber:
		switch v.(type) {
		case float64, float32, int, int64, int32, json.Number, uint64, uint32:
			return true
		default:
			return false
		}
	case model.ColBoolean:
		_, ok := v.(bool)
		return ok
	case model.ColTimestamp:
		switch vv := v.(type) {
		case string:
			_, err := time.Parse(time.RFC3339, vv)
			return err == nil
		default:
			return false
		}
	case model.ColJSON:
		return true
	default:
		return true
	}
}

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case fmt.Stringer:
		return x.String()
	case float64, float32, int, int64, int32, uint64, uint32, bool:
		return fmt.Sprint(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
