package httpx

import (
	"strings"
	"sync"

	"github.com/your/module/internal/model"
)

// RBACStore is an in-memory permission binding store (MVP) keyed by scope.
type RBACStore struct {
	mu       sync.RWMutex
	bindings map[string][]model.PermissionBinding // scope -> bindings
}

func NewRBACStore() *RBACStore { return &RBACStore{bindings: map[string][]model.PermissionBinding{}} }

// Grant adds or replaces a permission binding.
func (s *RBACStore) Grant(b model.PermissionBinding) {
	s.mu.Lock()
	defer s.mu.Unlock()
	arr := s.bindings[b.Scope]
	replaced := false
	for i, existing := range arr {
		if existing.Principal == b.Principal {
			arr[i] = b
			replaced = true
			break
		}
	}
	if !replaced {
		arr = append(arr, b)
	}
	s.bindings[b.Scope] = arr
}

// Revoke removes a permission binding for a principal at a given scope.
func (s *RBACStore) Revoke(scope, principal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	arr := s.bindings[scope]
	out := arr[:0]
	for _, b := range arr {
		if b.Principal == principal {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		delete(s.bindings, scope)
	} else {
		s.bindings[scope] = out
	}
}

// RoleFor returns the most specific role for principal (table scope overrides db scope). principal may be user:<id> or role:<name>.
func (s *RBACStore) RoleFor(principal string, tableID string, dbID string) model.Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// table first
	if tableID != "" {
		scope := "table:" + tableID
		for _, b := range s.bindings[scope] {
			if b.Principal == principal {
				return b.Role
			}
		}
	}
	if dbID != "" {
		scope := "db:" + dbID
		for _, b := range s.bindings[scope] {
			if b.Principal == principal {
				return b.Role
			}
		}
	}
	return "" // none
}

// Allow returns true if role permits action.
func Allow(role model.Role, action string) bool {
	if role == model.RoleAdmin {
		return true
	}
	switch action {
	case "db.create", "db.delete":
		return role == model.RoleMaintainer || role == model.RoleAdmin
	case "table.create", "table.schema", "table.delete":
		return role == model.RoleMaintainer || role == model.RoleAdmin
	case "row.read":
		return role == model.RoleViewer || role == model.RoleEditor || role == model.RoleMaintainer || role == model.RoleAdmin
	case "row.write":
		return role == model.RoleEditor || role == model.RoleMaintainer || role == model.RoleAdmin
	default:
		return false
	}
}

// MaskRow redacts masked columns for viewer/editor roles when column.Mask=true.
func MaskRow(role model.Role, schema []model.ColumnDef, row map[string]any) map[string]any {
	if role == model.RoleAdmin || role == model.RoleMaintainer {
		return row
	}
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	for _, c := range schema {
		if c.Mask && (role == model.RoleViewer || role == model.RoleEditor) {
			if _, ok := out[c.Name]; ok {
				out[c.Name] = "***"
			}
		}
	}
	return out
}

// PrincipalFromRequest is placeholder - in real system extract auth identity; for now returns empty unless X-Debug-Principal is set.
func PrincipalFromRequest(rh string) string {
	// future: parse auth headers; for dev allow header override X-Debug-Principal
	if strings.TrimSpace(rh) != "" {
		return rh
	}
	return ""
}
