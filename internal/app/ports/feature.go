package ports

import (
	"context"
	"encoding/json"
)

// FeatureChange: resultado de activar o suspender una funcionalidad en un tenant.
type FeatureChange struct {
	NewlyEnabled      bool
	TenantID          uint64
	TenantSlug        string
	Name, Description map[string]string
	Module, Status    string
	Roles             []string
}

// FeatureNew: funcionalidad que llegó con un despliegue y el Super Admin aún no conoce.
type FeatureNew struct {
	Code, Module      string
	Name, Description map[string]string
}

type FeatureRepository interface {
	TenantFeatures(ctx context.Context, tenantID uint64) ([]string, error)
	List(ctx context.Context, m Meta) ([]json.RawMessage, error)
	SetStatus(ctx context.Context, m Meta, code, status string) error
	Tenants(ctx context.Context, m Meta, code string) ([]json.RawMessage, error)
	SetTenant(ctx context.Context, m Meta, tenantID, code string, enabled bool) (*FeatureChange, error)
	Unnoticed(ctx context.Context) ([]FeatureNew, error)
	Noticed(ctx context.Context, code string) (bool, error)
}
