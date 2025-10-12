package model

// Database / table / row modeling for the experimental Databases feature.
// These structs are intentionally lightweight DTOs for the HTTP API layer and
// do not embed RethinkDB driver types to keep the boundary clean.

// DatabaseInstance represents a logical (per-org) database. In RethinkDB we may
// implement this either as a naming prefix or a separate database (recommended)
// depending on multi-tenancy strategy. For the MVP we assume one RethinkDB
// cluster and per-org database names like: org_<orgID>.
type DatabaseInstance struct {
	ID            string `json:"id"`
	OrgID         string `json:"org_id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Replication   int    `json:"replication,omitempty"`
	Shards        int    `json:"shards,omitempty"`
	PrimaryRegion string `json:"primary_region,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// ColumnType enumerates supported primitive types.
type ColumnType string

const (
	ColString    ColumnType = "string"
	ColNumber    ColumnType = "number"
	ColBoolean   ColumnType = "boolean"
	ColTimestamp ColumnType = "timestamp"
	ColJSON      ColumnType = "json"
)

// ColumnDef describes a single column in a table schema.
type ColumnDef struct {
	Name     string     `json:"name"`
	Type     ColumnType `json:"type"`
	Required bool       `json:"required,omitempty"`
	Unique   bool       `json:"unique,omitempty"`
	Indexed  bool       `json:"indexed,omitempty"`
	Default  any        `json:"default,omitempty"`
	Enum     []string   `json:"enum,omitempty"`
	Regex    string     `json:"regex,omitempty"`
	Mask     bool       `json:"mask,omitempty"` // if true, viewers may have value redacted
}

// Table represents a logical collection (RethinkDB table) with a schema.
type Table struct {
	ID         string      `json:"id"`
	DatabaseID string      `json:"db_id"`
	Name       string      `json:"name"`
	PrimaryKey string      `json:"primary_key"`
	TTL        int         `json:"ttl,omitempty"` // seconds (0 = none)
	Schema     []ColumnDef `json:"schema"`
	CreatedAt  string      `json:"created_at,omitempty"`
}

// View represents a saved query (filters, sort, column selection) for a table.
type View struct {
	ID        string   `json:"id"`
	TableID   string   `json:"table_id"`
	Name      string   `json:"name"`
	Query     string   `json:"query"` // serialized filter expression / DSL
	Columns   []string `json:"columns,omitempty"`
	Sort      []string `json:"sort,omitempty"`    // e.g. ["col1:asc","col2:desc"]
	Filters   []string `json:"filters,omitempty"` // reserved (structured client filters)
	CreatedBy string   `json:"created_by,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
}

// AuditScope enumerates scope types.
type AuditScope string

const (
	ScopeDB    AuditScope = "db"
	ScopeTable AuditScope = "table"
	ScopeRow   AuditScope = "row"
)

// AuditEvent captures a change for compliance / restore.
type AuditEvent struct {
	ID      string     `json:"id"`
	Scope   AuditScope `json:"scope"`
	ScopeID string     `json:"scope_id"`
	Actor   string     `json:"actor"`
	Action  string     `json:"action"`         // e.g. create_db, update_schema, insert_row, update_row, delete_row
	Diff    any        `json:"diff,omitempty"` // bounded diff representation
	TS      string     `json:"ts"`
}

// Role enumerates RBAC roles.
type Role string

const (
	RoleAdmin      Role = "admin"      // org-wide admin
	RoleMaintainer Role = "maintainer" // database or table maintainer
	RoleEditor     Role = "editor"     // can edit rows but not schema
	RoleViewer     Role = "viewer"     // read-only
)

// PermissionBinding associates a principal with a role on a scope.
type PermissionBinding struct {
	Principal string `json:"principal"` // user:<id> or role:<name>
	Scope     string `json:"scope"`     // db:<dbID> or table:<tableID>
	Role      Role   `json:"role"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ChangefeedEvent is emitted to realtime subscribers.
type ChangefeedEvent struct {
	Type     string `json:"type"` // init|insert|update|delete|snapshot|error
	TableID  string `json:"table_id"`
	RowID    string `json:"row_id,omitempty"`
	Before   any    `json:"before,omitempty"`
	After    any    `json:"after,omitempty"`
	Cursor   string `json:"cursor,omitempty"` // resume token (monotonic increasing logical sequence)
	TS       string `json:"ts"`
	Pending  int    `json:"pending,omitempty"` // backlog size when paused
	Snapshot bool   `json:"snapshot,omitempty"`
	Error    string `json:"error,omitempty"`
}

// QueryPage generic paginated payload wrapper.
type QueryPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	Total      int64  `json:"total,omitempty"`
}

// TableQueryRequest holds server-side filter/sort/pagination; kept generic for MVP.
type TableQueryRequest struct {
	ViewID   string         `json:"view_id,omitempty"`
	Filters  map[string]any `json:"filters,omitempty"`
	Sort     []string       `json:"sort,omitempty"`
	PageSize int            `json:"page_size,omitempty"`
	Cursor   string         `json:"cursor,omitempty"`
	Columns  []string       `json:"columns,omitempty"` // projection
}

// ImportPreviewRow shows a mapped row during dry-run import.
type ImportPreviewRow struct {
	Raw    map[string]any `json:"raw"`
	Mapped map[string]any `json:"mapped"`
	Errors []string       `json:"errors,omitempty"`
}

// ImportPreviewResponse bundles preview results.
type ImportPreviewResponse struct {
	Rows      []ImportPreviewRow `json:"rows"`
	ColumnMap map[string]string  `json:"column_map"`
	Total     int                `json:"total"`
	Errors    []string           `json:"errors,omitempty"`
}

// ExportRequest for exporting rows.
type ExportRequest struct {
	Format  string   `json:"format"`
	ViewID  string   `json:"view_id,omitempty"`
	Columns []string `json:"columns,omitempty"`
}
