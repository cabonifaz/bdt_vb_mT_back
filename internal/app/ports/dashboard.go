package ports

import (
	"context"
	"encoding/json"
)

// DashboardQuery: período (fechas locales), agrupación WEEK o MONTH, cliente opcional y la fecha local de hoy.
type DashboardQuery struct {
	From, To, Grain, Today string
	ClientID               *string
}

type DashboardRepository interface {
	Selection(ctx context.Context, m Meta, tenantID uint64, q DashboardQuery) (json.RawMessage, error)
}
