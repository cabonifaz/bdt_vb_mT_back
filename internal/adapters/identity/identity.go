// Package identity implementa la consulta de DNI contra proveedores públicos de datos de RENIEC.
// El proveedor y su token se configuran por variables de entorno (DNI_API_PROVIDER, DNI_API_TOKEN, DNI_API_URL).
//   - decolecta (por defecto): nombres y apellidos.
//   - factiliza: nombres, apellidos y fecha de nacimiento.
package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

var (
	errNotConfigured = domain.NewError("IDENTITY_NOT_CONFIGURED")
	errNotFound      = domain.NewError("IDENTITY_NOT_FOUND")
	errUnavailable   = domain.NewError("IDENTITY_UNAVAILABLE")
)

type Config struct {
	Provider string // decolecta | factiliza
	Token    string
	BaseURL  string // opcional: sobrescribe la URL del proveedor (con %s para el número)
}

// New devuelve el adaptador configurado; sin token responde IDENTITY_NOT_CONFIGURED.
func New(c Config) ports.IdentityLookup {
	hc := &http.Client{Timeout: 8 * time.Second}
	if strings.EqualFold(strings.TrimSpace(c.Provider), "factiliza") {
		return &client{name: "factiliza", token: c.Token, url: or(c.BaseURL, "https://api.factiliza.com/v1/dni/info/%s"), http: hc, parse: parseFactiliza}
	}
	return &client{name: "decolecta", token: c.Token, url: or(c.BaseURL, "https://api.decolecta.com/v1/reniec/dni?numero=%s"), http: hc, parse: parseDecolecta}
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

type client struct {
	name, token, url string
	http             *http.Client
	parse            func([]byte) (*domain.PersonIdentity, error)
}

func (c *client) Provider() string { return c.name }

func (c *client) LookupDNI(ctx context.Context, dni string) (*domain.PersonIdentity, error) {
	if c.token == "" {
		return nil, errNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(c.url, dni), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errUnavailable
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusUnprocessableEntity:
		return nil, errNotFound
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return nil, errNotConfigured
	case res.StatusCode != http.StatusOK:
		return nil, errUnavailable
	}
	p, err := c.parse(body)
	if err != nil {
		return nil, errUnavailable
	}
	if p == nil || (p.FirstName == "" && p.LastName == "") {
		return nil, errNotFound
	}
	p.DocumentNumber, p.Provider = dni, c.name
	p.FirstName, p.LastName, p.SecondLastName = title(p.FirstName), title(p.LastName), title(p.SecondLastName)
	return p, nil
}

func parseDecolecta(b []byte) (*domain.PersonIdentity, error) {
	var r struct {
		FirstName      string `json:"first_name"`
		FirstLastName  string `json:"first_last_name"`
		SecondLastName string `json:"second_last_name"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &domain.PersonIdentity{FirstName: r.FirstName, LastName: r.FirstLastName, SecondLastName: r.SecondLastName}, nil
}

func parseFactiliza(b []byte) (*domain.PersonIdentity, error) {
	var r struct {
		Success *bool `json:"success"`
		Data    struct {
			Nombres         string `json:"nombres"`
			ApellidoPaterno string `json:"apellido_paterno"`
			ApellidoMaterno string `json:"apellido_materno"`
			FechaNacimiento string `json:"fecha_nacimiento"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Success != nil && !*r.Success {
		return nil, nil
	}
	p := &domain.PersonIdentity{FirstName: r.Data.Nombres, LastName: r.Data.ApellidoPaterno, SecondLastName: r.Data.ApellidoMaterno}
	if d := NormDate(r.Data.FechaNacimiento); d != "" {
		p.BirthDate = &d
	}
	return p, nil
}

// NormDate acepta YYYY-MM-DD o DD/MM/YYYY y devuelve YYYY-MM-DD ("" si no es válida).
func NormDate(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// title: "MARIA DEL PILAR" -> "Maria Del Pilar" (RENIEC devuelve mayúsculas).
func title(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
