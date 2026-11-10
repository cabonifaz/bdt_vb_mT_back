// Package igdt: cliente de la API de integración de IGDT (Ingreso y Gestión de Talentos), que gobierna los catálogos
// compartidos con BDT. Cada llamada usa el token del tenant (X-Api-Key); el token nunca aparece en logs ni errores.
package igdt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Client: BaseURL es la URL de la API de IGDT del ambiente (IGDT_API_URL). Sin BaseURL la integración no está disponible.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
			// sin redirecciones: el token solo viaja al host configurado
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) Enabled() bool { return c != nil && c.BaseURL != "" }

const maxBody = 16 << 20 // una página de cambios o el mapeo de la carga inicial

func (c *Client) do(ctx context.Context, method, path, key string, in, out any) error {
	if !c.Enabled() {
		return domain.NewError("IGDT_NOT_CONFIGURED")
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bdt-igdt-sync/1")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.NewError("IGDT_UNAVAILABLE"), errors.Unwrap(err))
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return domain.NewError("IGDT_UNAVAILABLE")
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return domain.NewError("IGDT_KEY_INVALID")
	case res.StatusCode == http.StatusForbidden:
		return domain.NewError("IGDT_SCOPE_MISSING")
	case res.StatusCode >= 300:
		return fmt.Errorf("%w: %w", domain.NewError("IGDT_UNAVAILABLE"), httpStatus(res.StatusCode))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: respuesta inválida", domain.NewError("IGDT_UNAVAILABLE"))
	}
	return nil
}

func (c *Client) WhoAmI(ctx context.Context, key string) (*ports.IGDTIdentity, error) {
	var out ports.IGDTIdentity
	if err := c.do(ctx, http.MethodGet, "/integration/v1/whoami", key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Changes(ctx context.Context, key string, since uint64, limit int) (*ports.IGDTChanges, error) {
	q := url.Values{"since": {strconv.FormatUint(since, 10)}, "limit": {strconv.Itoa(limit)}}
	var out ports.IGDTChanges
	if err := c.do(ctx, http.MethodGet, "/integration/v1/catalog/changes?"+q.Encode(), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Import(ctx context.Context, key string, export json.RawMessage) (json.RawMessage, error) {
	var out struct {
		Map json.RawMessage `json:"map"`
	}
	if err := c.do(ctx, http.MethodPost, "/integration/v1/catalog/import", key, export, &out); err != nil {
		return nil, err
	}
	return out.Map, nil
}

func (c *Client) push(ctx context.Context, path, key string, items []json.RawMessage) ([]ports.IGDTPushResult, error) {
	var out struct {
		Results []ports.IGDTPushResult `json:"results"`
	}
	if err := c.do(ctx, http.MethodPost, path, key, map[string]any{"items": items}, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// PushRequirements publica en IGDT el último estado de los RQ (bandeja de selección).
func (c *Client) PushRequirements(ctx context.Context, key string, items []json.RawMessage) ([]ports.IGDTPushResult, error) {
	return c.push(ctx, "/integration/v1/requirements", key, items)
}

// PushHires envía los candidatos confirmados (o el retiro de su confirmación): solicitudes de ingreso en IGDT.
func (c *Client) PushHires(ctx context.Context, key string, items []json.RawMessage) ([]ports.IGDTPushResult, error) {
	return c.push(ctx, "/integration/v1/hires", key, items)
}

// HireEvents lee los ingresos registrados en IGDT desde el cursor (alcance hires.read).
func (c *Client) HireEvents(ctx context.Context, key string, since uint64, limit int) (*ports.IGDTHireEvents, error) {
	q := url.Values{"since": {strconv.FormatUint(since, 10)}, "limit": {strconv.Itoa(limit)}}
	var out ports.IGDTHireEvents
	if err := c.do(ctx, http.MethodGet, "/integration/v1/hires/events?"+q.Encode(), key, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LinkBDT entrega a IGDT la llave BDT automática del tenant, con la que IGDT lee las fichas y los CV de los candidatos.
// Un IGDT que aún no tiene el endpoint responde 404 (IGDT_LINK_UNSUPPORTED): se reintenta más tarde.
func (c *Client) LinkBDT(ctx context.Context, key, bdtKey string) error {
	return notFound(c.do(ctx, http.MethodPut, "/integration/v1/bdt-link", key, map[string]string{"apiKey": bdtKey}, nil))
}

// UnlinkBDT retira de IGDT la llave BDT (al desconectar).
func (c *Client) UnlinkBDT(ctx context.Context, key string) error {
	return notFound(c.do(ctx, http.MethodDelete, "/integration/v1/bdt-link", key, nil, nil))
}

// httpStatus: código HTTP de una respuesta fallida de IGDT (se envuelve junto a IGDT_UNAVAILABLE).
type httpStatus int

func (h httpStatus) Error() string { return "http " + strconv.Itoa(int(h)) }

func notFound(err error) error {
	var h httpStatus
	if errors.As(err, &h) && h == http.StatusNotFound {
		return domain.NewError("IGDT_LINK_UNSUPPORTED")
	}
	return err
}
