package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// DocumentService: documentos de postulación. Maestro del tenant, documentos que presenta cada postulante y su
// validación: manual (analista, jefe o admin) o automática para el CUL, contra el servicio del MTPE con el RUC
// del tenant. Un CUL es auténtico si el PDF subido es idéntico (misma huella) al PDF oficial del MTPE.
// Faltantes o sin validar solo se avisan: no bloquean las etapas del postulante.
type DocumentService struct {
	Repo   ports.DocumentRepository
	Cipher ports.Cipher
	Files  FileUploader
	CUL    ports.CULVerifier
}

// FileUploader guarda el archivo cifrado en el talento (lo implementa TalentService).
type FileUploader interface {
	UploadFile(ctx context.Context, p *domain.Principal, ip, talentID, category, name string, r io.Reader) (*domain.TalentFile, error)
}

// CULHint: datos que el navegador leyó del QR del CUL (código del certificado y documento del titular).
type CULHint struct {
	Code      string `json:"culCode"`
	DocNumber string `json:"culDoc"`
}

const defaultCULValidityDays = 90 // el CUL vence a los 3 meses de su emisión

var (
	culCodeRe = regexp.MustCompile(`^\d{11}$`)
	docNumRe  = regexp.MustCompile(`^[0-9A-Za-z]{8,12}$`)
)

func (s *DocumentService) Rules(ctx context.Context, p *domain.Principal) ([]json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	return s.Repo.Rules(ctx, meta(p, ""), tid)
}

func (s *DocumentService) SaveRule(ctx context.Context, p *domain.Principal, ip string, r ports.DocRule) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(r.TypeID) || (r.DefaultMode != "REQUIRED" && r.DefaultMode != "OPTIONAL") ||
		(r.Verification != "MANUAL" && r.Verification != "AUTO_CUL") || r.SortOrder < 0 || r.SortOrder > 10_000 ||
		(r.ValidityDays != nil && (*r.ValidityDays < 1 || *r.ValidityDays > 3650)) {
		return domain.ErrValidation
	}
	return s.Repo.SaveRule(ctx, meta(p, ip), tid, r)
}

// CandidateDetail: ficha del postulante dentro del RQ (documentos pedidos con su estado y línea de tiempo).
func (s *DocumentService) CandidateDetail(ctx context.Context, p *domain.Principal, candidateID, today string) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(candidateID) {
		return nil, domain.ErrNotFound
	}
	doc, err := s.Repo.CandidateDetail(ctx, meta(p, ""), tid, candidateID, boardToday(today, time.Now()))
	if err != nil {
		return nil, err
	}
	return s.decryptNames(doc), nil
}

// Upload guarda el archivo en el talento, lo registra para el tipo pedido y, si el tipo se valida
// automáticamente (CUL), lo verifica en el momento.
func (s *DocumentService) Upload(ctx context.Context, p *domain.Principal, ip, candidateID, typeID, name string, r io.Reader, hint CULHint, today string) (*ports.DocVerdict, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(candidateID) || !validUUID(typeID) {
		return nil, domain.ErrValidation
	}
	m := meta(p, ip)
	talentID, err := s.Repo.CandidateTalent(ctx, m, tid, candidateID)
	if err != nil {
		return nil, err
	}
	f, err := s.Files.UploadFile(ctx, p, ip, talentID, "DOCUMENT", name, r)
	if err != nil {
		return nil, err
	}
	dc, err := s.Repo.Create(ctx, m, tid, uuid.NewString(), candidateID, typeID, f.ID, boardToday(today, time.Now()))
	if err != nil {
		return nil, err
	}
	if dc.Verification != "AUTO_CUL" {
		return &ports.DocVerdict{Status: "PENDING"}, nil
	}
	return s.verifyCUL(ctx, p, ip, tid, candidateID, dc, hint)
}

// Recheck vuelve a consultar el CUL (p. ej. si el MTPE no respondió o se completó el RUC del tenant).
func (s *DocumentService) Recheck(ctx context.Context, p *domain.Principal, ip, candidateID, docID string, hint CULHint) (*ports.DocVerdict, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(candidateID) || !validUUID(docID) {
		return nil, domain.ErrNotFound
	}
	dc, err := s.Repo.Context(ctx, meta(p, ip), tid, docID)
	if err != nil {
		return nil, err
	}
	if dc.Verification != "AUTO_CUL" {
		return nil, domain.ErrValidation
	}
	return s.verifyCUL(ctx, p, ip, tid, candidateID, dc, hint)
}

// Verify: validación manual (aprobar o rechazar con motivo).
func (s *DocumentService) Verify(ctx context.Context, p *domain.Principal, ip, candidateID, docID string, approve bool, reason string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if !validUUID(candidateID) || !validUUID(docID) || utf8.RuneCountInString(reason) > 500 || (!approve && reason == "") {
		return domain.ErrValidation
	}
	v := ports.DocVerdict{Status: "REJECTED", Method: "MANUAL", Reason: reason}
	if approve {
		v.Status = "VERIFIED"
	}
	return s.Repo.Verify(ctx, meta(p, ip), tid, docID, candidateID, v)
}

func (s *DocumentService) verifyCUL(ctx context.Context, p *domain.Principal, ip string, tid uint64, candidateID string, dc *ports.DocContext, hint CULHint) (*ports.DocVerdict, error) {
	talentDoc := ""
	if len(dc.TalentDocEnc) > 0 {
		talentDoc, _ = s.Cipher.Decrypt(dc.TalentDocEnc)
	}
	docType, docNum, early := culRequest(dc, talentDoc, hint)
	v := early
	if v == nil {
		var chk *ports.CULCheck
		var err error
		if s.CUL == nil {
			err = errors.New("cul: verificador no configurado")
		} else {
			chk, err = s.CUL.Check(ctx, dc.TenantRUC, hint.Code, docType, docNum)
		}
		if err != nil && !errors.Is(err, ports.ErrCULDocType) {
			slog.Warn("no se pudo consultar el CUL en el MTPE", "err", err)
		}
		v = culVerdict(dc, hint.Code, docNum, chk, err, time.Now())
	}
	if err := s.Repo.Verify(ctx, meta(p, ip), tid, dc.ID, candidateID, *v); err != nil {
		return nil, err
	}
	return v, nil
}

// culRequest decide con qué datos consultar al MTPE, o devuelve el veredicto si no se puede consultar.
// El documento del titular sale del QR; si el talento tiene documento registrado, deben coincidir.
func culRequest(dc *ports.DocContext, talentDoc string, hint CULHint) (docType, docNum string, early *ports.DocVerdict) {
	pending := func(reason string) *ports.DocVerdict {
		return &ports.DocVerdict{Status: "PENDING", Method: "AUTO", Reason: reason}
	}
	if strings.TrimSpace(dc.TenantRUC) == "" {
		return "", "", pending("TENANT_RUC_MISSING")
	}
	if !culCodeRe.MatchString(hint.Code) {
		return "", "", pending("CUL_CODE_MISSING")
	}
	docNum = strings.TrimSpace(hint.DocNumber)
	if docNum == "" {
		docNum = talentDoc
	}
	if !docNumRe.MatchString(docNum) {
		return "", "", pending("CUL_DOC_MISSING")
	}
	if talentDoc != "" && !strings.EqualFold(talentDoc, docNum) {
		return "", "", &ports.DocVerdict{Status: "REJECTED", Method: "AUTO", Reason: "CUL_OTHER_PERSON"}
	}
	docType = dc.TalentDocType
	if docType == "" || docType == "PASSPORT" {
		docType = "DNI"
	}
	return docType, docNum, nil
}

// culVerdict: auténtico solo si el MTPE lo tiene, está vigente y el PDF subido es idéntico al oficial.
func culVerdict(dc *ports.DocContext, code, docNum string, chk *ports.CULCheck, err error, now time.Time) *ports.DocVerdict {
	v := &ports.DocVerdict{Method: "AUTO"}
	switch {
	case errors.Is(err, ports.ErrCULDocType):
		v.Status, v.Reason = "PENDING", "CUL_DOC_TYPE"
	case err != nil || chk == nil:
		v.Status, v.Reason = "PENDING", "CUL_UNREACHABLE"
	case !chk.Found:
		v.Status, v.Reason = "REJECTED", "CUL_NOT_FOUND"
	case !strings.EqualFold(chk.OfficialSHA256, dc.FileSHA256):
		v.Status, v.Reason = "REJECTED", "CUL_FILE_ALTERED"
	case !chk.Vigente:
		v.Status, v.Reason = "REJECTED", "CUL_EXPIRED"
	default:
		v.Status = "VERIFIED"
	}
	res := map[string]any{"code": code, "docNumber": docNum, "checkedAt": now.UTC().Format(time.RFC3339)}
	if chk != nil && chk.Found {
		res["names"], res["vigente"], res["match"] = chk.Names, chk.Vigente, strings.EqualFold(chk.OfficialSHA256, dc.FileSHA256)
		if !chk.IssuedAt.IsZero() {
			days := defaultCULValidityDays
			if dc.ValidityDays != nil {
				days = *dc.ValidityDays
			}
			issued := chk.IssuedAt
			expires := issued.AddDate(0, 0, days)
			v.IssuedAt, v.ExpiresAt = &issued, &expires
			res["issuedAt"] = issued.Format(time.RFC3339)
		}
	} else if chk != nil {
		res["message"] = chk.Message
	}
	v.Result, _ = json.Marshal(res)
	return v
}

// decryptNames reemplaza los campos "<x>NameEnc" (nombre cifrado en base64) por "<x>Name" descifrado, a cualquier profundidad.
func (s *DocumentService) decryptNames(doc json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(doc, &v) != nil {
		return doc
	}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, val := range t {
				if enc, ok := val.(string); ok && strings.HasSuffix(k, "NameEnc") {
					name := ""
					if b, err := base64.StdEncoding.DecodeString(enc); err == nil {
						name, _ = s.Cipher.Decrypt(b)
					}
					delete(t, k)
					t[strings.TrimSuffix(k, "Enc")] = name
					continue
				}
				walk(val)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return doc
	}
	return out
}
