// Package cul consulta el Certificado Único Laboral (CUL) en el servicio público de verificación del MTPE
// (empleosperu.gob.pe): la misma consulta que hace la página a la que lleva el QR del certificado.
// La consulta exige el RUC de la empresa que verifica (el del tenant). El servicio no es una API documentada:
// cualquier respuesta inesperada se informa como error para que el documento quede pendiente de verificar.
package cul

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

const defaultBase = "https://www.empleosperu.gob.pe/CertificadoUnicoLaboral/"

// Tipos de documento del MTPE (combo de la página de verificación).
var docTypes = map[string]string{"DNI": "10080399", "CE": "10080305", "CPP": "10080307"}

type MTPE struct {
	Client  *http.Client
	BaseURL string // vacío = servicio público del MTPE
}

func New() *MTPE { return &MTPE{Client: &http.Client{Timeout: 25 * time.Second}} }

func (m *MTPE) base() string {
	if m.BaseURL != "" {
		return m.BaseURL
	}
	return defaultBase
}

// Check busca el certificado y, si existe, descarga el PDF oficial para comparar su huella con la del archivo subido.
func (m *MTPE) Check(ctx context.Context, ruc, code, docType, docNumber string) (*ports.CULCheck, error) {
	tipo, ok := docTypes[docType]
	if !ok {
		return nil, ports.ErrCULDocType
	}
	q := url.Values{"numeroCertijoven": {code}, "numeroRUC": {ruc}, "tipoDocumento": {tipo}, "numeroDNI": {docNumber}}
	var found struct {
		Estado  int    `json:"estadoRespuesta"`
		Mensaje string `json:"mensajeRespuesta"`
		T       *struct {
			Emision   string `json:"a_fecoperac"`
			DocToken  string `json:"vNuDocNacimiento"`
			Secuencia int    `json:"a_numsecuen"`
			ApPat     string `json:"v_apellpat"`
			ApMat     string `json:"v_apellmat"`
			Nombres   string `json:"v_nombres"`
			Numero    string `json:"a_numdocume"`
			Vigente   int    `json:"esVigente"`
		} `json:"T"`
	}
	if err := m.getJSON(ctx, "buscarCertiJoven.html?"+q.Encode(), 64<<10, &found); err != nil {
		return nil, err
	}
	if found.Estado != 2 || found.T == nil {
		return &ports.CULCheck{Found: false, Message: strings.TrimSpace(found.Mensaje)}, nil
	}
	t := found.T
	out := &ports.CULCheck{Found: true, Code: t.Numero, Vigente: t.Vigente == 1,
		Names: strings.Join(strings.Fields(t.Nombres+" "+t.ApPat+" "+t.ApMat), " ")}
	if at, err := time.ParseInLocation("02/01/2006 15:04:05", t.Emision, lima); err == nil {
		out.IssuedAt = at
	}
	// PDF oficial: arreglo de bytes (con signo) dentro de un JSON
	var pdf struct {
		T []int `json:"T"`
	}
	pq := url.Values{"numeroDni": {t.DocToken}, "tipoDocumento": {tipo}, "numeroSecuencia": {fmt.Sprint(t.Secuencia)}}
	if err := m.getJSON(ctx, "VerPDF.html?"+pq.Encode(), 16<<20, &pdf); err != nil {
		return nil, err
	}
	if len(pdf.T) == 0 {
		return nil, errors.New("cul: el MTPE no devolvió el PDF oficial")
	}
	h := sha256.New()
	buf := make([]byte, len(pdf.T))
	for i, b := range pdf.T {
		buf[i] = byte(b)
	}
	h.Write(buf)
	out.OfficialSHA256 = hex.EncodeToString(h.Sum(nil))
	return out, nil
}

var lima = time.FixedZone("America/Lima", -5*3600)

func (m *MTPE) getJSON(ctx context.Context, path string, limit int64, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.base()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("cul: el MTPE respondió %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return errors.New("cul: respuesta demasiado grande")
	}
	return json.Unmarshal(latin1(body), v)
}

// latin1: el MTPE responde en ISO-8859-1 (nombres con Ñ o tildes); se pasa a UTF-8 si hace falta.
func latin1(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out := make([]rune, len(b))
	for i, c := range b {
		out[i] = rune(c)
	}
	return []byte(string(out))
}
