package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

func (s *Server) talentRoutes(r chi.Router) {
	read := s.requirePerm("talent.read")
	write := s.requirePerm("talent.write")
	r.Use(s.idempotent)

	// búsqueda por POST: los filtros (nombres, criterios) no quedan en URLs ni en logs de acceso
	r.With(read).Post("/talents/search", s.searchTalents)
	r.With(write).Post("/talents", s.saveTalent)
	r.With(read).Get("/talents/files/{fid}", s.downloadTalentFile)
	r.With(write).Delete("/talents/files/{fid}", s.deleteTalentFile)
	r.With(read).Delete("/talents/feedback/{fid}", s.deleteFeedback)
	r.With(write).Delete("/talents/sections/{section}/{itemId}", s.deleteTalentSection)
	r.With(read).Get("/talents/{id}", s.getTalent)
	r.With(write).Put("/talents/{id}", s.saveTalent)
	r.With(write).Patch("/talents/{id}/status", s.setTalentStatus)
	r.With(write).Patch("/talents/{id}/availability", s.setTalentAvailability)
	r.With(write).Patch("/talents/{id}/equipment", s.setTalentEquipment)
	r.With(write).Put("/talents/{id}/tech-skills", s.saveTechSkill)
	r.With(write).Post("/talents/{id}/soft-skills", s.addSoftSkill)
	r.With(write).Put("/talents/{id}/experiences", s.saveExperience)
	r.With(write).Put("/talents/{id}/educations", s.saveEducation)
	r.With(write).Put("/talents/{id}/languages", s.saveLanguage)
	r.With(read).Put("/talents/{id}/feedback", s.saveFeedback)
	r.With(write).Post("/talents/{id}/files", s.uploadTalentFile)
	r.With(read, s.requirePerm("talent.cv.generate")).Post("/talents/{id}/cv/translate", s.translateCV)

	r.With(read).Get("/collections", s.listCollections)
	r.With(read).Post("/collections", s.createCollection)
	r.With(read).Patch("/collections/{id}", s.renameCollection)
	r.With(read).Delete("/collections/{id}", s.deleteCollection)
	r.With(read).Get("/collections/{id}/shares", s.collectionShares)
	r.With(read).Put("/collections/{id}/shares/{userId}", s.shareCollection)
	r.With(read).Delete("/collections/{id}/shares/{userId}", s.unshareCollection)
	r.With(read).Get("/colleagues", s.listColleagues)
	r.With(read).Put("/collections/{id}/talents/{tid}", s.addToCollection)
	r.With(read).Delete("/collections/{id}/talents/{tid}", s.removeFromCollection)
}

// idempotent: si la petición trae X-Client-Op-Id (operación encolada offline), se registra
// y un reenvío de la misma operación no se vuelve a aplicar.
func (s *Server) idempotent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := r.Header.Get("X-Client-Op-Id")
		p := principal(r)
		if op == "" || r.Method == http.MethodGet || p == nil || p.TenantID == nil {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := uuid.Parse(op); err != nil {
			s.writeErr(w, r, domain.ErrValidation)
			return
		}
		entity := uuidRe.ReplaceAllString(strings.TrimPrefix(r.URL.Path, "/api/v1/tenant/"), "{id}")
		if len(entity) > 60 {
			entity = entity[:60]
		}
		dup, status, result, err := s.Sync.Register(r.Context(), *p.TenantID, p.UserID, op, entity, r.Method)
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		if dup {
			w.Header().Set("X-Idempotent-Replay", "true")
			writeJSON(w, http.StatusOK, map[string]any{"replayed": true, "status": status, "result": json.RawMessage(nonEmpty(result))})
			return
		}
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		st := "APPLIED"
		if rec.status >= 400 {
			st = "REJECTED"
		}
		res, _ := json.Marshal(map[string]any{"httpStatus": rec.status})
		_ = s.Sync.Complete(r.Context(), *p.TenantID, op, st, res)
	})
}

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func nonEmpty(b []byte) []byte {
	if len(b) == 0 {
		return []byte("null")
	}
	return b
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (s *Server) searchTalents(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Search       string   `json:"search"`
		Skills       []string `json:"skills"`
		EnglishLevel string   `json:"englishLevel"`
		MinYears     *float64 `json:"minYears"`
		Position     string   `json:"position"`
		Education    string   `json:"education"`
		GradeID      string   `json:"gradeId"`
		CollectionID string   `json:"collectionId"`
		Status       string   `json:"status"`
		Page         int      `json:"page"`
		PageSize     int      `json:"pageSize"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if len(in.Skills) > 30 {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	page, err := s.Talents.Search(r.Context(), principal(r), ports.TalentFilter{
		Search: in.Search, Skills: strings.Join(in.Skills, ","), EnglishLevel: in.EnglishLevel, MinYears: in.MinYears,
		Position: in.Position, Education: in.Education, GradeID: in.GradeID, CollectionID: in.CollectionID, Status: in.Status,
		Page: in.Page, PageSize: in.PageSize,
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getTalent(w http.ResponseWriter, r *http.Request) {
	t, err := s.Talents.Get(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) saveTalent(w http.ResponseWriter, r *http.Request) {
	var in app.TalentInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		in.ID = id
	}
	res, err := s.Talents.Save(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

// POST /talents/{id}/cv/translate {"lang":"en"}: textos del CV traducidos para generar el PDF con la marca del tenant.
func (s *Server) translateCV(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Lang string `json:"lang"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Talents.TranslateCV(r.Context(), principal(r), chi.URLParam(r, "id"), strings.ToLower(in.Lang))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setTalentStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Talents.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), strings.ToUpper(in.Status)); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setTalentAvailability(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AvailabilityID *string `json:"availabilityId"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SetAvailability(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.AvailabilityID))
}

func (s *Server) setTalentEquipment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HasEquipment *bool `json:"hasEquipment"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SetEquipment(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.HasEquipment))
}

func (s *Server) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) saveTechSkill(w http.ResponseWriter, r *http.Request) {
	var in app.SkillInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SaveTechSkill(r.Context(), principal(r), chi.URLParam(r, "id"), in))
}

func (s *Server) addSoftSkill(w http.ResponseWriter, r *http.Request) {
	var in app.SkillInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.AddSoftSkill(r.Context(), principal(r), chi.URLParam(r, "id"), in))
}

func (s *Server) saveExperience(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        string  `json:"id"`
		Company   string  `json:"company"`
		Position  string  `json:"position"`
		StartDate string  `json:"startDate"`
		EndDate   *string `json:"endDate"`
		IsCurrent bool    `json:"isCurrent"`
		Functions *string `json:"functions"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SaveExperience(r.Context(), principal(r), chi.URLParam(r, "id"), domain.TalentExperience{
		ID: in.ID, Company: in.Company, Position: in.Position, StartDate: in.StartDate, EndDate: in.EndDate,
		IsCurrent: in.IsCurrent, Functions: in.Functions}))
}

func (s *Server) saveEducation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID            string  `json:"id"`
		Institution   string  `json:"institution"`
		Career        string  `json:"career"`
		GradeID       *string `json:"gradeId"`
		StartDate     *string `json:"startDate"`
		EndDate       *string `json:"endDate"`
		IsCurrent     bool    `json:"isCurrent"`
		DatePrecision string  `json:"datePrecision"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SaveEducation(r.Context(), principal(r), chi.URLParam(r, "id"), domain.TalentEducation{
		ID: in.ID, Institution: in.Institution, Career: in.Career, GradeID: in.GradeID, StartDate: in.StartDate,
		EndDate: in.EndDate, IsCurrent: in.IsCurrent, DatePrecision: in.DatePrecision}))
}

func (s *Server) saveLanguage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID         string `json:"id"`
		LanguageID string `json:"languageId"`
		LevelID    string `json:"levelId"`
		Stars      int    `json:"stars"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.SaveLanguage(r.Context(), principal(r), chi.URLParam(r, "id"), domain.TalentLanguage{
		ID: in.ID, LanguageID: in.LanguageID, LevelID: in.LevelID, Stars: in.Stars}))
}

func (s *Server) deleteTalentSection(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.DeleteSection(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "section"), chi.URLParam(r, "itemId")))
}

func (s *Server) saveFeedback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID      string `json:"id"`
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	avg, err := s.Talents.SaveFeedback(r.Context(), principal(r), chi.URLParam(r, "id"), in.ID, in.Stars, in.Comment)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avgRating": avg})
}

func (s *Server) deleteFeedback(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.DeleteFeedback(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "fid")))
}

var fileCategories = map[string]bool{"PHOTO": true, "CV": true, "CV_TENANT_ES": true, "CV_TENANT_EN": true, "CERTIFICATE": true, "OTHER": true}

func (s *Server) uploadTalentFile(w http.ResponseWriter, r *http.Request) {
	cat := strings.ToUpper(r.URL.Query().Get("category"))
	if !fileCategories[cat] {
		s.writeErr(w, r, domain.NewError("INVALID_KIND"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		s.writeErr(w, r, domain.NewError("FILE_TOO_LARGE"))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	defer f.Close()
	out, err := s.Talents.UploadFile(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), cat, hdr.Filename, f)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) downloadTalentFile(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.Talents.OpenFile(r.Context(), principal(r), chi.URLParam(r, "fid"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	disp := "attachment"
	if r.URL.Query().Get("inline") == "1" && (f.MimeType == "application/pdf" || strings.HasPrefix(f.MimeType, "image/")) {
		disp = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", f.MimeType)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.OriginalName}))
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-store")
	h.Set("ETag", fmt.Sprintf(`"%s"`, f.SHA256))
	_, _ = bytes.NewReader(data).WriteTo(w)
}

func (s *Server) deleteTalentFile(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.DeleteFile(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "fid")))
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	list, err := s.Talents.Collections(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Talents.CreateCollection(r.Context(), principal(r), in.ID, in.Name)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.DeleteCollection(r.Context(), principal(r), chi.URLParam(r, "id")))
}

func (s *Server) addToCollection(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.SetCollectionTalent(r.Context(), principal(r), chi.URLParam(r, "id"), chi.URLParam(r, "tid"), true))
}

func (s *Server) removeFromCollection(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.SetCollectionTalent(r.Context(), principal(r), chi.URLParam(r, "id"), chi.URLParam(r, "tid"), false))
}

func (s *Server) renameCollection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Talents.RenameCollection(r.Context(), principal(r), chi.URLParam(r, "id"), in.Name))
}

func (s *Server) collectionShares(w http.ResponseWriter, r *http.Request) {
	list, err := s.Talents.CollectionShares(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) shareCollection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Permission string `json:"permission"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Talents.ShareCollection(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), chi.URLParam(r, "userId"), in.Permission)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) unshareCollection(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Talents.UnshareCollection(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), chi.URLParam(r, "userId")))
}

func (s *Server) listColleagues(w http.ResponseWriter, r *http.Request) {
	list, err := s.Talents.Colleagues(r.Context(), principal(r), r.URL.Query().Get("q"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
