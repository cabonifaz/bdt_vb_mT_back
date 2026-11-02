package ports

import (
	"context"
	"encoding/json"
)

type VersionRepository interface {
	Register(ctx context.Context, version, env, status, commit string, notes json.RawMessage) (bool, error)
	Current(ctx context.Context, env, version string) (json.RawMessage, error)
	List(ctx context.Context, env string) ([]json.RawMessage, error)
	SetStatus(ctx context.Context, m Meta, env, version, status string) error
}
