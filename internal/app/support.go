package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// SupportService: solicitudes de los usuarios al Super Admin (observaciones, incidencias y mejoras) con el origen
// (pantalla desde donde se pidió), referencias e imágenes; la atención del Super Admin; las novedades de la
// plataforma y las guías de primer uso vistas por cada usuario.
// Notifica al Super Admin cada solicitud o respuesta del usuario, al usuario cada respuesta del Super Admin y a
// todos los usuarios cada novedad.
type SupportService struct {
	Repo     ports.SupportRepository
	Cipher   ports.Cipher
	Storage  ports.FileStorage
	Notifier *NotificationService
}

const maxSupportFile = 5 << 20 // 5 MB por imagen o archivo

var (
	supportKinds   = map[string]bool{"INCIDENT": true, "OBSERVATION": true, "IMPROVEMENT": true}
	supportImpacts = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true}
	supportStatus  = map[string]bool{"OPEN": true, "IN_PROGRESS": true, "ANSWERED": true, "CLOSED": true}
	supportMimes   = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif", "application/pdf": "pdf"}
	tourKeyRe      = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)
)

// SupportInput: lo que envía el usuario. Origin lo arma el navegador (pantalla, ruta, módulo, entidad, navegador).
type SupportInput struct {
	Kind   string          `json:"kind"`
	Impact string          `json:"impact"`
	Title  string          `json:"title"`
	Body   string          `json:"body"`
	Origin json.RawMessage `json:"origin"`
	Refs   []string        `json:"refs"`
	Files  []string        `json:"files"`
}

// UploadFile guarda una imagen o PDF cifrado; queda suelto hasta que se adjunta a una solicitud o respuesta.
func (s *SupportService) UploadFile(ctx context.Context, p *domain.Principal, ip, name string, r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSupportFile+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxSupportFile {
		return "", domain.NewError("FILE_TOO_LARGE")
	}
	if len(data) == 0 {
		return "", domain.ErrValidation
	}
	mime := http.DetectContentType(data)
	if i := strings.Index(mime, ";"); i > 0 {
		mime = mime[:i]
	}
	ext, ok := supportMimes[mime]
	if !ok {
		return "", domain.NewError("FILE_TYPE_NOT_ALLOWED")
	}
	ct, err := s.Cipher.EncryptBytes(data)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	scope := "platform"
	if p.TenantUUID != "" {
		scope = "tenants/" + p.TenantUUID
	}
	key := path.Join(scope, "support", id+"."+ext+".enc")
	if _, _, err := s.Storage.Put(ctx, key, bytes.NewReader(ct), int64(len(ct))); err != nil {
		return "", err
	}
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if utf8.RuneCountInString(base) > 200 {
		base = string([]rune(base)[:200])
	}
	err = s.Repo.CreateFile(ctx, meta(p, ip), p.TenantID, domain.FileObject{UUID: id, StorageKey: key, OriginalName: base,
		MimeType: mime, Size: int64(len(data)), SHA256: sha256Hex(data), Encrypted: true})
	if err != nil {
		_ = s.Storage.Delete(ctx, key)
		return "", err
	}
	return id, nil
}

// Create registra la solicitud (solo usuarios de un tenant) y avisa al Super Admin.
func (s *SupportService) Create(ctx context.Context, p *domain.Principal, ip string, in SupportInput) (string, int, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return "", 0, err
	}
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if in.Impact == "" {
		in.Impact = "MEDIUM"
	}
	origin, ok := cleanOrigin(in.Origin)
	if !supportKinds[in.Kind] || !supportImpacts[in.Impact] || in.Title == "" || utf8.RuneCountInString(in.Title) > 200 ||
		in.Body == "" || utf8.RuneCountInString(in.Body) > 10_000 || !ok || !validFiles(in.Files) {
		return "", 0, domain.ErrValidation
	}
	refs, ok := cleanRefs(in.Refs)
	if !ok {
		return "", 0, domain.ErrValidation
	}
	id := uuid.NewString()
	number, err := s.Repo.Create(ctx, meta(p, ip), tid, ports.SupportNew{ID: id, MessageID: uuid.NewString(), Kind: in.Kind,
		Impact: in.Impact, Title: in.Title, Body: in.Body, Origin: origin, Refs: refs, Files: filesJSON(in.Files)})
	if err != nil {
		return "", 0, err
	}
	payload, _ := json.Marshal(map[string]any{"number": number, "title": in.Title, "kind": in.Kind, "impact": in.Impact, "tenant": p.TenantSlug})
	s.notifyUsers(ctx, p, 0, domain.NotifSupportNew, "SUPPORT", id, payload, ports.AudienceSuperAdmins, 0, func(actor, locale string) pushMessage {
		return supportPush(locale, true, false, actor, number, in.Title, "/soporte/"+id)
	})
	return id, number, nil
}

// Reply: respuesta del Super Admin (con cambio de estado opcional) o del autor; avisa al otro lado.
func (s *SupportService) Reply(ctx context.Context, p *domain.Principal, ip, id, body, status string, files []string) (*ports.SupportReplied, error) {
	body = strings.TrimSpace(body)
	if !validUUID(id) || utf8.RuneCountInString(body) > 10_000 || (status != "" && !supportStatus[status]) || !validFiles(files) {
		return nil, domain.ErrValidation
	}
	r, err := s.Repo.Reply(ctx, meta(p, ip), id, uuid.NewString(), body, status, filesJSON(files))
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{"number": r.Number, "title": r.Title, "status": r.Status})
	if r.Staff {
		s.notifyUsers(ctx, p, r.TenantID, domain.NotifSupportReply, "SUPPORT", id, payload, ports.AudienceUser, r.ReporterID, func(actor, locale string) pushMessage {
			return supportPush(locale, false, true, actor, r.Number, r.Title, "/soporte/"+id)
		})
	} else {
		s.notifyUsers(ctx, p, 0, domain.NotifSupportReply, "SUPPORT", id, payload, ports.AudienceSuperAdmins, 0, func(actor, locale string) pushMessage {
			return supportPush(locale, false, false, actor, r.Number, r.Title, "/soporte/"+id)
		})
	}
	return r, nil
}

func (s *SupportService) List(ctx context.Context, p *domain.Principal, status, search string) ([]json.RawMessage, error) {
	if status != "" && status != "ACTIVE" && !supportStatus[status] || utf8.RuneCountInString(search) > 100 {
		return nil, domain.ErrValidation
	}
	docs, err := s.Repo.List(ctx, meta(p, ""), status, strings.TrimSpace(search), 300)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i] = decryptNames(s.Cipher, docs[i])
	}
	return docs, nil
}

func (s *SupportService) Get(ctx context.Context, p *domain.Principal, id string) (json.RawMessage, error) {
	if !validUUID(id) {
		return nil, domain.ErrNotFound
	}
	doc, err := s.Repo.Get(ctx, meta(p, ""), id)
	if err != nil {
		return nil, err
	}
	return decryptNames(s.Cipher, doc), nil
}

// OpenFile: imagen o archivo de una solicitud (solo su autor o el Super Admin).
func (s *SupportService) OpenFile(ctx context.Context, p *domain.Principal, fileID string) (*domain.FileObject, []byte, error) {
	if !validUUID(fileID) {
		return nil, nil, domain.ErrNotFound
	}
	f, err := s.Repo.File(ctx, meta(p, ""), fileID)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, err
	}
	if f.Encrypted {
		if data, err = s.Cipher.DecryptBytes(data); err != nil {
			return nil, nil, err
		}
	}
	return f, data, nil
}

// ReleaseInput: novedad en español e inglés; Module y TourKey enlazan la pantalla y su guía.
type ReleaseInput struct {
	Title   map[string]string `json:"title"`
	Body    map[string]string `json:"body"`
	Module  string            `json:"module"`
	TourKey string            `json:"tourKey"`
}

// PublishRelease publica una novedad y la notifica a todos los usuarios de la plataforma.
func (s *SupportService) PublishRelease(ctx context.Context, p *domain.Principal, ip string, in ReleaseInput) (string, error) {
	es, en := strings.TrimSpace(in.Title["es"]), strings.TrimSpace(in.Title["en"])
	bes, ben := strings.TrimSpace(in.Body["es"]), strings.TrimSpace(in.Body["en"])
	if es == "" || bes == "" || utf8.RuneCountInString(es) > 200 || utf8.RuneCountInString(en) > 200 ||
		utf8.RuneCountInString(bes) > 5000 || utf8.RuneCountInString(ben) > 5000 || len(in.Module) > 40 ||
		(in.TourKey != "" && !tourKeyRe.MatchString(in.TourKey)) {
		return "", domain.ErrValidation
	}
	if en == "" {
		en = es
	}
	if ben == "" {
		ben = bes
	}
	id := uuid.NewString()
	title, _ := json.Marshal(map[string]string{"es": es, "en": en})
	body, _ := json.Marshal(map[string]string{"es": bes, "en": ben})
	if err := s.Repo.PublishRelease(ctx, meta(p, ip), id, title, body, in.Module, in.TourKey); err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]any{"title": map[string]string{"es": es, "en": en}, "module": in.Module})
	s.notifyUsers(ctx, p, 0, domain.NotifReleaseNote, "RELEASE", id, payload, ports.AudienceAll, 0, func(_, locale string) pushMessage {
		t, h := es, "Novedad en el Banco de Talentos"
		if strings.HasPrefix(locale, "en") {
			t, h = en, "What's new in Talent Bank"
		}
		return pushMessage{Title: h, Body: t, URL: "/novedades", Tag: "release-" + id}
	})
	return id, nil
}

func (s *SupportService) Releases(ctx context.Context, p *domain.Principal) ([]json.RawMessage, error) {
	return s.Repo.Releases(ctx, meta(p, ""), 50)
}

func (s *SupportService) Tours(ctx context.Context, p *domain.Principal) (map[string]int, error) {
	return s.Repo.Tours(ctx, meta(p, ""))
}

func (s *SupportService) TourSeen(ctx context.Context, p *domain.Principal, key string, version int) error {
	if !tourKeyRe.MatchString(key) || version < 1 || version > 1000 {
		return domain.ErrValidation
	}
	return s.Repo.TourSeen(ctx, meta(p, ""), key, version)
}

// notifyUsers registra la notificación y envía el push en segundo plano; nunca hace fallar la operación.
func (s *SupportService) notifyUsers(ctx context.Context, p *domain.Principal, tid uint64, typ, entity, entityID string, payload json.RawMessage,
	audience string, userID uint64, push func(actor, locale string) pushMessage) {
	n := s.Notifier
	if n == nil {
		return
	}
	in := ports.NotifyInput{ID: uuid.NewString(), Type: typ, EntityType: entity, EntityID: entityID, Payload: payload}
	res, err := n.Repo.NotifyUsers(context.WithoutCancel(ctx), meta(p, ""), tid, in, audience, userID)
	if err != nil {
		slog.Warn("no se pudo registrar la notificación", "type", typ, "err", err)
		return
	}
	if len(res.Targets) == 0 || n.Push == nil || !n.Push.Enabled() {
		return
	}
	actor, _ := n.Cipher.Decrypt(res.ActorNameEnc)
	go n.deliver(res.ID, res.Targets, func(locale string) pushMessage {
		m := push(actor, locale)
		m.ID = res.ID
		return m
	})
}

// supportPush: texto del push de soporte en el idioma del destinatario.
func supportPush(locale string, created, fromStaff bool, actor string, number int, title, link string) pushMessage {
	en := strings.HasPrefix(locale, "en")
	h := map[bool]string{true: "Nueva solicitud de soporte", false: "Respuesta en la solicitud"}[created]
	if fromStaff {
		h = "El equipo de soporte respondió"
	}
	if en {
		h = map[bool]string{true: "New support request", false: "Reply on the request"}[created]
		if fromStaff {
			h = "Support replied"
		}
	}
	body := "#" + strconv.Itoa(number) + " · " + title
	if actor != "" && !fromStaff {
		body = actor + ": " + body
	}
	return pushMessage{Title: h, Body: body, URL: link, Tag: "support-" + strconv.Itoa(number)}
}

// cleanOrigin: objeto JSON pequeño con datos de la pantalla (lo arma el navegador).
func cleanOrigin(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil, false
	}
	out, _ := json.Marshal(m)
	return out, true
}

// cleanRefs: hasta 10 enlaces o referencias de texto.
func cleanRefs(refs []string) (json.RawMessage, bool) {
	var out []string
	for _, r := range refs {
		if r = strings.TrimSpace(r); r == "" {
			continue
		}
		if utf8.RuneCountInString(r) > 500 {
			return nil, false
		}
		out = append(out, r)
	}
	if len(out) > 10 {
		return nil, false
	}
	if len(out) == 0 {
		return nil, true
	}
	b, _ := json.Marshal(out)
	return b, true
}

func validFiles(files []string) bool {
	if len(files) > 10 {
		return false
	}
	for _, f := range files {
		if !validUUID(f) {
			return false
		}
	}
	return true
}

func filesJSON(files []string) json.RawMessage {
	if len(files) == 0 {
		return nil
	}
	b, _ := json.Marshal(files)
	return b
}
