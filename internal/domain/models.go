package domain

import (
	"encoding/json"
	"time"
)

type Tenant struct {
	ID            uint64    `json:"-"`
	UUID          string    `json:"id"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	LegalName     *string   `json:"legalName,omitempty"`
	TaxID         *string   `json:"taxId,omitempty"`
	Status        string    `json:"status"`
	DefaultLocale string    `json:"defaultLocale"`
	Timezone      string    `json:"timezone"`
	UserCount     int       `json:"userCount"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Branding es la configuración visual pública de un tenant.
type Branding struct {
	TenantID        uint64    `json:"-"`
	TenantUUID      string    `json:"tenantId"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	DefaultLocale   string    `json:"defaultLocale"`
	DisplayName     string    `json:"displayName"`
	LogoURL         string    `json:"logoUrl"`
	LogoDarkURL     string    `json:"logoDarkUrl,omitempty"`
	FaviconURL      string    `json:"faviconUrl,omitempty"`
	ColorPrimary    string    `json:"colorPrimary"`
	ColorPrimaryHov string    `json:"colorPrimaryHover"`
	ColorSecondary  string    `json:"colorSecondary"`
	ColorAccent     string    `json:"colorAccent"`
	ColorSidebar    string    `json:"colorSidebar"`
	FontFamily      string    `json:"fontFamily"`
	Version         time.Time `json:"version"`
	// Plantilla del CV con la marca del tenant
	CVHeaderURL  string   `json:"cvHeaderUrl,omitempty"`
	CVFooterURL  string   `json:"cvFooterUrl,omitempty"`
	CVFooterText string   `json:"cvFooterText,omitempty"`
	CVShowPhoto  bool     `json:"cvShowPhoto"`
	CVStyle      *CVStyle `json:"cvStyle,omitempty"`
}

// CVStyle: formato del CV adoptado desde un CV de ejemplo (PDF) del tenant.
type CVStyle struct {
	NameAlign      string `json:"nameAlign"`              // left | center
	NameColor      string `json:"nameColor,omitempty"`    // #RRGGBB
	HeadingColor   string `json:"headingColor,omitempty"` // #RRGGBB
	HeadingRule    bool   `json:"headingRule"`            // línea bajo los títulos de sección
	SummaryHeading bool   `json:"summaryHeading"`         // el resumen lleva título propio
	BoldPositions  bool   `json:"boldPositions"`          // puesto y fechas en negrita
	FooterBg       string `json:"footerBg,omitempty"`     // recuadro del pie (#RRGGBB)
	Font           string `json:"font"`                   // Helvetica | Times-Roman
}

type User struct {
	ID          uint64     `json:"-"`
	UUID        string     `json:"id"`
	Email       string     `json:"email"`
	FullName    string     `json:"fullName"`
	Locale      string     `json:"locale"`
	Status      string     `json:"status"`
	Roles       []string   `json:"roles"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// AuthUser es el registro que se usa para verificar credenciales.
type AuthUser struct {
	ID           uint64
	UUID         string
	TenantID     *uint64
	TenantUUID   string
	TenantSlug   string
	TenantStatus string
	EmailEnc     []byte
	FullNameEnc  []byte
	PasswordHash string
	Locale       string
	Status       string
	LockedUntil  *time.Time
}

type Invitation struct {
	UUID       string     `json:"id"`
	Email      string     `json:"email"`
	RoleCode   string     `json:"role"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	TenantSlug string     `json:"tenantSlug,omitempty"`
	TenantName string     `json:"tenantName,omitempty"`
}

type ApiKey struct {
	UUID       string     `json:"id"`
	TenantSlug string     `json:"tenantSlug"`
	TenantName string     `json:"tenantName"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	Status     string     `json:"status"`
	EnabledAt  *time.Time `json:"enabledAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	// Secret solo se devuelve una vez, al crearla.
	Secret string `json:"secret,omitempty"`
}

type ApiKeyRecord struct {
	ID           uint64
	UUID         string
	TenantID     uint64
	TenantSlug   string
	Hash         string
	Scopes       []string
	Status       string
	ExpiresAt    *time.Time
	TenantStatus string
}

type MasterTable struct {
	Code        string          `json:"code"`
	Name        json.RawMessage `json:"name"`
	Description *string         `json:"description,omitempty"`
	Scope       string          `json:"scope"`
	IsSystem    bool            `json:"isSystem"`
	ParentCode  *string         `json:"parentCode,omitempty"`
}

type MasterItem struct {
	UUID          string          `json:"id"`
	TableCode     string          `json:"table"`
	Code          string          `json:"code"`
	Label         json.RawMessage `json:"label"`
	ParentUUID    *string         `json:"parentId,omitempty"`
	Num1          *float64        `json:"num1,omitempty"`
	Num2          *float64        `json:"num2,omitempty"`
	Num3          *float64        `json:"num3,omitempty"`
	Str1          *string         `json:"str1,omitempty"`
	Str2          *string         `json:"str2,omitempty"`
	Str3          *string         `json:"str3,omitempty"`
	SortOrder     int             `json:"sortOrder"`
	IsActive      bool            `json:"isActive"`
	IsTenantOwned bool            `json:"isTenantOwned"`
	LegacyID      *int            `json:"legacyId,omitempty"`
}

type FileObject struct {
	ID           uint64
	UUID         string
	TenantID     *uint64
	OwnerType    string
	Category     string
	StorageKey   string
	OriginalName string
	MimeType     string
	Size         int64
	SHA256       string
	Encrypted    bool
}

type AuditEntry struct {
	ID        uint64          `json:"id"`
	UserUUID  *string         `json:"userId,omitempty"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *string         `json:"entityId,omitempty"`
	Detail    json.RawMessage `json:"detail,omitempty"`
	IP        *string         `json:"ip,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Page[T any] struct {
	Total int `json:"total"`
	Items []T `json:"items"`
}
