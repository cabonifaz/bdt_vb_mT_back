package domain

// Tipos de consumo de servicios externos de pago.
const (
	UsageDNILookup   = "DNI_LOOKUP"
	UsageAITranslate = "AI_TRANSLATE"
	UsageAICVRead    = "AI_CV_READ"
)

// AIUsage: tokens consumidos en una llamada al modelo de IA.
type AIUsage struct {
	Model            string
	PromptTokens     int64
	CompletionTokens int64
}

// UsageRow: consumo de un tenant en un mes, por tipo y proveedor (reporte del Super Admin).
type UsageRow struct {
	TenantID         string `json:"tenantId"`
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	Month            string `json:"month"` // YYYY-MM
	Kind             string `json:"kind"`
	Provider         string `json:"provider"`
	Calls            int64  `json:"calls"`
	Errors           int64  `json:"errors"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
}
