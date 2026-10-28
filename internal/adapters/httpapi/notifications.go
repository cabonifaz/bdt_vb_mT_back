package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Centro de notificaciones del usuario del tenant y suscripción push del dispositivo (todas sin caché).
func (s *Server) notificationRoutes(r chi.Router) {
	r.Get("/notifications", s.listNotifications)
	r.Post("/notifications/read", s.readNotifications)
	r.Get("/notifications/prefs", s.notificationPrefs)
	r.Put("/notifications/prefs", s.setNotificationPref)
	r.Get("/notifications/push/key", s.pushKey)
	r.Post("/notifications/push/subscribe", s.pushSubscribe)
	r.Post("/notifications/push/unsubscribe", s.pushUnsubscribe)
}

func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

// GET /notifications?unread=1&before=<RFC3339>&limit=20
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	out, err := s.Notifications.Inbox(r.Context(), principal(r), q.Get("unread") == "1", q.Get("before"), limit)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

// POST /notifications/read {"id":"..."} (sin id: todas)
func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Notifications.MarkRead(r.Context(), principal(r), in.ID); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) notificationPrefs(w http.ResponseWriter, r *http.Request) {
	out, err := s.Notifications.Prefs(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setNotificationPref(w http.ResponseWriter, r *http.Request) {
	var in domain.NotificationPref
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Notifications.SetPref(r.Context(), principal(r), in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /notifications/push/key: clave pública VAPID (enabled = false si el servidor no tiene push configurado)
func (s *Server) pushKey(w http.ResponseWriter, r *http.Request) {
	key, ok := s.Notifications.PushKey()
	noStore(w)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": ok, "publicKey": key})
}

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	var in app.PushSubscription
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	n, err := s.Notifications.Subscribe(r.Context(), principal(r), in, r.UserAgent())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"devices": n})
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Notifications.Unsubscribe(r.Context(), principal(r), in.Endpoint); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
