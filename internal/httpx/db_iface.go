package httpx

import (
	"context"

	"github.com/your/module/internal/db"
	"github.com/your/module/internal/model"
)

// DBManager captures the subset of database manager methods used by handlers.
type DBManager interface {
	ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error)
	CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error)
	GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error)
	DeleteDatabase(ctx context.Context, orgID, dbID string) error

	GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error)
	CreateTable(ctx context.Context, orgID, dbID string, t model.Table) error
	UpdateTableSchema(ctx context.Context, orgID, dbID, table string, schema []model.ColumnDef, pk string) error

	QueryRows(ctx context.Context, orgID, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error)
	InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error)
	UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error
	DeleteRow(ctx context.Context, orgID, dbID, table, id string) error

	ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error)
	SubscribeTable(ctx context.Context, orgID, dbID, table string) (*db.ChangefeedStream, error)
	Ping(ctx context.Context) error
}
