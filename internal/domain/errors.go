// Package domain contiene las entidades y reglas del núcleo, sin dependencias externas.
package domain

import (
	"errors"
	"net/http"
)

// Error es un error de negocio con código estable (la UI lo traduce con i18n: errors.<Code>).
type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string { return e.Code }

func NewError(code string) *Error {
	return &Error{Code: code, Status: statusFor(code)}
}

var (
	ErrUnauthorized       = NewError("UNAUTHORIZED")
	ErrForbidden          = NewError("FORBIDDEN")
	ErrNotFound           = NewError("NOT_FOUND")
	ErrInvalidCredentials = NewError("INVALID_CREDENTIALS")
	ErrAccountLocked      = NewError("ACCOUNT_LOCKED")
	ErrAccountDisabled    = NewError("ACCOUNT_DISABLED")
	ErrTenantInactive     = NewError("TENANT_INACTIVE")
	ErrValidation         = NewError("VALIDATION")
	ErrWeakPassword       = NewError("WEAK_PASSWORD")
	ErrRateLimited        = NewError("RATE_LIMITED")
	ErrInternal           = NewError("INTERNAL")
)

func statusFor(code string) int {
	switch code {
	case "UNAUTHORIZED", "INVALID_CREDENTIALS", "SESSION_INVALID", "SESSION_EXPIRED", "SESSION_REUSED", "APIKEY_INVALID":
		return http.StatusUnauthorized
	case "FORBIDDEN", "TENANT_MISMATCH", "ROLE_NOT_ALLOWED", "MASTER_SCOPE", "ACCOUNT_LOCKED", "ACCOUNT_DISABLED",
		"TENANT_INACTIVE", "APIKEY_NOT_ENABLED", "APIKEY_SCOPE":
		return http.StatusForbidden
	case "NOT_FOUND", "FILE_NOT_FOUND", "ROLE_NOT_FOUND", "PARENT_NOT_FOUND", "TALENT_NOT_FOUND", "INTERVIEW_NOT_FOUND",
		"IDENTITY_NOT_FOUND", "RQ_NOT_FOUND":
		return http.StatusNotFound
	case "IDENTITY_NOT_CONFIGURED", "AI_NOT_CONFIGURED":
		return http.StatusServiceUnavailable
	case "IDENTITY_UNAVAILABLE", "AI_UNAVAILABLE":
		return http.StatusBadGateway
	case "TENANT_SLUG_TAKEN", "USER_ALREADY_EXISTS", "CODE_TAKEN", "SUPERADMIN_EXISTS", "LAST_TENANT_ADMIN",
		"APIKEY_REVOKED", "INVITATION_INVALID", "INVITATION_EXPIRED", "VERSION_CONFLICT", "TALENT_EMAIL_EXISTS",
		"TALENT_DOC_EXISTS", "COLLECTION_EXISTS", "BLACKLIST_EXISTS", "BLACKLIST_GLOBAL_EXISTS", "TALENT_BLACKLISTED",
		"RQ_CODE_EXISTS", "CANDIDATE_EXISTS", "VACANCY_HAS_CANDIDATES", "VACANCY_FULL", "RQ_CLOSED", "RQ_HAS_CONFIRMED", "VACANCY_BELOW_CONFIRMED":
		return http.StatusConflict
	case "RATE_LIMITED":
		return http.StatusTooManyRequests
	case "INTERNAL":
		return http.StatusInternalServerError
	}
	return http.StatusUnprocessableEntity
}

// AsError devuelve el error de dominio o INTERNAL.
func AsError(err error) *Error {
	var de *Error
	if errors.As(err, &de) {
		return de
	}
	return ErrInternal
}
