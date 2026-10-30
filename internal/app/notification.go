package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// notifyGroupMinutes: cambios seguidos del mismo usuario sobre el mismo talento se agrupan en una notificación.
const notifyGroupMinutes = 15

// inboxScope: tenant del usuario, o 0 para el Super Admin (su bandeja tiene las notificaciones de plataforma).
func inboxScope(p *domain.Principal) (uint64, error) {
	if p != nil && p.IsPlatform() {
		return 0, nil
	}
	return tenantOf(p)
}

// NotificationService: centro de notificaciones del usuario y push del sistema operativo (app instalada).
type NotificationService struct {
	Repo   ports.NotificationRepository
	Push   ports.PushSender
	Cipher ports.Cipher
}

func (s *NotificationService) Inbox(ctx context.Context, p *domain.Principal, unreadOnly bool, before string, limit int) (*domain.NotificationInbox, error) {
	tid, err := inboxScope(p)
	if err != nil {
		return nil, err
	}
	var b *time.Time
	if before != "" {
		t, err := time.Parse(time.RFC3339Nano, before)
		if err != nil {
			return nil, domain.ErrValidation
		}
		b = &t
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	unread, recs, err := s.Repo.List(ctx, meta(p, ""), tid, unreadOnly, b, limit)
	if err != nil {
		return nil, err
	}
	out := &domain.NotificationInbox{Unread: unread, Items: make([]domain.Notification, 0, len(recs))}
	for _, r := range recs {
		n := r.Notification
		if r.ActorID != "" {
			name, _ := s.Cipher.Decrypt(r.ActorNameEnc)
			n.Actor = &domain.NotificationActor{ID: r.ActorID, Name: name}
		}
		out.Items = append(out.Items, n)
	}
	return out, nil
}

// MarkRead marca una notificación (id) o todas (id vacío).
func (s *NotificationService) MarkRead(ctx context.Context, p *domain.Principal, id string) error {
	tid, err := inboxScope(p)
	if err != nil {
		return err
	}
	var ref *string
	if id != "" {
		if !validUUID(id) {
			return domain.ErrValidation
		}
		ref = &id
	}
	return s.Repo.MarkRead(ctx, meta(p, ""), tid, ref)
}

func (s *NotificationService) PushKey() (string, bool) {
	if s.Push == nil || !s.Push.Enabled() {
		return "", false
	}
	return s.Push.PublicKey(), true
}

// PushSubscription: lo que entrega PushManager.subscribe() del navegador.
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func endpointHash(e string) string {
	h := sha256.Sum256([]byte(e))
	return hex.EncodeToString(h[:])
}

// validEndpoint: solo URLs https de servicios push (evita usar el servidor para llamar a direcciones arbitrarias).
func validEndpoint(e string) bool {
	u, err := url.Parse(e)
	if err != nil || u.Scheme != "https" || u.Host == "" || len(e) > 1000 {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host != "localhost" && !strings.HasPrefix(host, "127.") && !strings.HasPrefix(host, "10.") &&
		!strings.HasPrefix(host, "192.168.") && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal")
}

func (s *NotificationService) Subscribe(ctx context.Context, p *domain.Principal, sub PushSubscription, userAgent string) (int, error) {
	tid, err := inboxScope(p)
	if err != nil {
		return 0, err
	}
	if !validEndpoint(sub.Endpoint) || sub.Keys.P256dh == "" || len(sub.Keys.P256dh) > 200 || sub.Keys.Auth == "" || len(sub.Keys.Auth) > 100 {
		return 0, domain.ErrValidation
	}
	if r := []rune(userAgent); len(r) > 300 {
		userAgent = string(r[:300])
	}
	return s.Repo.Subscribe(ctx, meta(p, ""), tid, endpointHash(sub.Endpoint), sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth, userAgent)
}

func (s *NotificationService) Unsubscribe(ctx context.Context, p *domain.Principal, endpoint string) error {
	tid, err := inboxScope(p)
	if err != nil {
		return err
	}
	if endpoint == "" {
		return domain.ErrValidation
	}
	return s.Repo.Unsubscribe(ctx, meta(p, ""), tid, endpointHash(endpoint))
}

// Prefs: configuración de cada tipo (sin fila = todo activo) y cantidad de dispositivos con push.
func (s *NotificationService) Prefs(ctx context.Context, p *domain.Principal) (*domain.NotificationPrefs, error) {
	tid, err := inboxScope(p)
	if err != nil {
		return nil, err
	}
	saved, devices, err := s.Repo.Prefs(ctx, meta(p, ""), tid)
	if err != nil {
		return nil, err
	}
	out := &domain.NotificationPrefs{Devices: devices}
	for _, t := range domain.NotificationTypes {
		pref := domain.NotificationPref{Type: t, InApp: true, Push: true}
		for _, x := range saved {
			if x.Type == t {
				pref = x
			}
		}
		out.Types = append(out.Types, pref)
	}
	return out, nil
}

func (s *NotificationService) SetPref(ctx context.Context, p *domain.Principal, pref domain.NotificationPref) error {
	tid, err := inboxScope(p)
	if err != nil {
		return err
	}
	if !domain.ValidNotificationType(pref.Type) {
		return domain.ErrValidation
	}
	return s.Repo.SetPref(ctx, meta(p, ""), tid, pref)
}

// TalentSaved notifica el alta o la actualización de un talento a los usuarios del tenant que pueden verlo.
// Nunca hace fallar el guardado: los errores solo se registran.
func (s *NotificationService) TalentSaved(ctx context.Context, p *domain.Principal, talentID string, created bool, name, headline string) {
	tid, err := tenantOf(p)
	if err != nil {
		return
	}
	typ, group := domain.NotifTalentUpdated, domain.NotifTalentCreated // una edición justo después del alta se agrupa con el alta
	if created {
		typ, group = domain.NotifTalentCreated, ""
	}
	payload, _ := json.Marshal(map[string]string{"name": name, "headline": headline})
	link := "/" + p.TenantSlug + "/talentos/" + talentID
	s.notify(ctx, p, tid, ports.NotifyInput{Type: typ, GroupWith: group, EntityType: "TALENT", EntityID: talentID,
		Payload: payload, RecipientPerm: "talent.read"}, func(actor, locale, id string) pushMessage {
		return talentPush(locale, created, actor, name, headline, link, id)
	})
}

// RQChanged notifica el alta de un RQ o sus cambios (datos o estado) a los usuarios del tenant que ven requerimientos.
func (s *NotificationService) RQChanged(ctx context.Context, p *domain.Principal, c *ports.RQChange) {
	tid, err := tenantOf(p)
	if err != nil {
		return
	}
	typ, group := domain.NotifRQUpdated, domain.NotifRQCreated
	if c.Created {
		typ, group = domain.NotifRQCreated, ""
	}
	payload, _ := json.Marshal(map[string]string{"code": c.Code, "title": c.Title, "client": c.Client, "status": c.Status, "previousStatus": c.PreviousStatus})
	link := "/" + p.TenantSlug + "/requerimientos/" + c.ID
	s.notify(ctx, p, tid, ports.NotifyInput{Type: typ, GroupWith: group, EntityType: "REQUIREMENT", EntityID: c.ID,
		Payload: payload, RecipientPerm: "rq.read"}, func(actor, locale, id string) pushMessage {
		return rqPush(locale, c, actor, link, id)
	})
}

// notify registra la notificación (agrupando cambios seguidos) y, si es nueva, envía el push en segundo plano.
// Nunca hace fallar la operación del usuario: los errores solo se registran.
func (s *NotificationService) notify(ctx context.Context, p *domain.Principal, tid uint64, in ports.NotifyInput, push func(actor, locale, id string) pushMessage) {
	in.ID, in.GroupMinutes = uuid.NewString(), notifyGroupMinutes
	res, err := s.Repo.Notify(context.WithoutCancel(ctx), meta(p, ""), tid, in)
	if err != nil {
		slog.Warn("no se pudo registrar la notificación", "type", in.Type, "err", err)
		return
	}
	if !res.New || len(res.Targets) == 0 || s.Push == nil || !s.Push.Enabled() {
		return
	}
	actor, _ := s.Cipher.Decrypt(res.ActorNameEnc)
	go s.deliver(res.ID, res.Targets, func(locale string) pushMessage { return push(actor, locale, res.ID) })
}

var rqStatusLabel = map[string][2]string{
	"REGISTERED": {"Registrado", "Registered"}, "ASSIGNED": {"Asignado", "Assigned"}, "IN_SELECTION": {"En selección", "In selection"},
	"IN_PRODUCTION": {"A procesar", "To process"}, "ATTENDED": {"Atendido", "Fulfilled"}, "FINISHED": {"Terminado", "Finished"},
	"LOST": {"Perdido", "Lost"}, "CANCELLED": {"Cancelado", "Cancelled"},
}

// rqPush arma el texto del push de un RQ en el idioma del destinatario.
func rqPush(locale string, c *ports.RQChange, actor, link, id string) pushMessage {
	en := 0
	title, verb := "Requerimiento actualizado", "actualizó"
	if c.Created {
		title, verb = "Nuevo requerimiento", "registró"
	}
	if strings.HasPrefix(locale, "en") {
		en = 1
		title, verb = "Requirement updated", "updated"
		if c.Created {
			title, verb = "New requirement", "added"
		}
	}
	body := strings.TrimSpace(actor + " " + verb + " " + c.Code + " · " + c.Title)
	if c.Client != "" {
		body += " (" + c.Client + ")"
	}
	if !c.Created && c.PreviousStatus != "" && c.PreviousStatus != c.Status {
		body += " → " + rqStatusLabel[c.Status][en]
	}
	return pushMessage{Title: title, Body: body, URL: link, Tag: "rq-" + c.ID, ID: id}
}

type pushMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	ID    string `json:"id"`
}

// talentPush arma el texto del push en el idioma del destinatario.
func talentPush(locale string, created bool, actor, name, headline, link, id string) pushMessage {
	en := strings.HasPrefix(locale, "en")
	title := map[bool]string{true: "Nuevo talento", false: "Talento actualizado"}[created]
	verb := map[bool]string{true: "registró a", false: "actualizó a"}[created]
	if en {
		title = map[bool]string{true: "New talent", false: "Talent updated"}[created]
		verb = map[bool]string{true: "added", false: "updated"}[created]
	}
	body := strings.TrimSpace(actor + " " + verb + " " + name)
	if headline != "" {
		body += " · " + headline
	}
	return pushMessage{Title: title, Body: body, URL: link, Tag: "talent-" + link, ID: id}
}

// deliver envía el push a cada dispositivo (en segundo plano) y depura las suscripciones caducadas.
func (s *NotificationService) deliver(id string, targets []ports.PushTarget, msg func(locale string) pushMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, t := range targets {
		body, _ := json.Marshal(msg(t.Locale))
		gone, err := s.Push.Send(ctx, t, body)
		if err != nil {
			slog.Warn("push no entregado", "notification", id, "err", err)
		}
		if rerr := s.Repo.PushResult(ctx, t.Hash, err == nil && !gone, gone); rerr != nil {
			slog.Warn("no se pudo registrar el resultado del push", "err", rerr)
		}
	}
}
