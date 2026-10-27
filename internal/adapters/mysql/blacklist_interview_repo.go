package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// ---------------------------------------------------------------- Lista negra

type BlacklistRepo struct{ *Store }

func scanEntries(rs *resultSets) ([]ports.BlacklistEntryRecord, error) {
	out := []ports.BlacklistEntryRecord{}
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			var e ports.BlacklistEntryRecord
			var clientID sql.NullString
			var client []byte
			if err := scan(&e.ID, &clientID, &client, &e.Reason, &e.ChangedAt, &e.UserNameEnc); err != nil {
				return err
			}
			e.ClientID = strPtr(clientID)
			if len(client) > 0 {
				e.Client = json.RawMessage(client)
			}
			out = append(out, e)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}

func (r BlacklistRepo) Search(ctx context.Context, m ports.Meta, tenantID uint64, search, clientUUID string, globalOnly bool, page, size int) (*domain.Page[domain.BlacklistTalent], error) {
	rs, err := r.call(ctx, "sp_blacklist_list", m.ActorID, tenantID, nullStr(search), nullStr(clientUUID), globalOnly, page, size)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &domain.Page[domain.BlacklistTalent]{Items: []domain.BlacklistTalent{}}
	rs.Next()
	if err := rs.One(&out.Total); err != nil {
		return nil, err
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var t domain.BlacklistTalent
			var second, headline sql.NullString
			if err := scan(&t.TalentID, &t.FirstName, &t.LastName, &second, &headline, &t.LastChange, &t.IsGlobal, &t.ClientCount); err != nil {
				return err
			}
			t.SecondLastName, t.Headline = strPtr(second), strPtr(headline)
			out.Items = append(out.Items, t)
			return nil
		})
	}
	if err != nil {
		return nil, err
	}
	return out, rs.Close()
}

func (r BlacklistRepo) Status(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID string) ([]ports.BlacklistEntryRecord, error) {
	rs, err := r.call(ctx, "sp_blacklist_status", m.ActorID, tenantID, talentUUID)
	if err != nil {
		return nil, err
	}
	return scanEntries(rs)
}

func (r BlacklistRepo) History(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID string, page, size int) (int, []ports.BlacklistMovementRecord, error) {
	rs, err := r.call(ctx, "sp_blacklist_history", m.ActorID, tenantID, talentUUID, page, size)
	if err != nil {
		return 0, nil, err
	}
	defer rs.Close()
	var total int
	rs.Next()
	if err := rs.One(&total); err != nil {
		return 0, nil, err
	}
	out := []ports.BlacklistMovementRecord{}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var h ports.BlacklistMovementRecord
			var client []byte
			if err := scan(&h.ID, &h.Movement, &client, &h.Reason, &h.CreatedAt, &h.UserNameEnc); err != nil {
				return err
			}
			if len(client) > 0 {
				h.Client = json.RawMessage(client)
			}
			out = append(out, h)
			return nil
		})
	}
	if err != nil {
		return 0, nil, err
	}
	return total, out, rs.Close()
}

func (r BlacklistRepo) Create(ctx context.Context, m ports.Meta, tenantID uint64, uuid, talentUUID, clientUUID, reason string) ([]ports.BlacklistEntryRecord, error) {
	rs, err := r.call(ctx, "sp_blacklist_create", m.ActorID, tenantID, uuid, talentUUID, nullStr(clientUUID), reason, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return scanEntries(rs)
}

func (r BlacklistRepo) Update(ctx context.Context, m ports.Meta, tenantID uint64, uuid, reason string) ([]ports.BlacklistEntryRecord, error) {
	rs, err := r.call(ctx, "sp_blacklist_update", m.ActorID, tenantID, uuid, reason, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return scanEntries(rs)
}

func (r BlacklistRepo) Remove(ctx context.Context, m ports.Meta, tenantID uint64, uuid, reason string) ([]ports.BlacklistEntryRecord, error) {
	rs, err := r.call(ctx, "sp_blacklist_remove", m.ActorID, tenantID, uuid, reason, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return scanEntries(rs)
}

func (r BlacklistRepo) LiftGlobal(ctx context.Context, m ports.Meta, tenantID uint64, uuid, reason string, keep []ports.BlacklistKeep) ([]ports.BlacklistEntryRecord, error) {
	if keep == nil {
		keep = []ports.BlacklistKeep{}
	}
	k, _ := json.Marshal(keep)
	rs, err := r.call(ctx, "sp_blacklist_lift_global", m.ActorID, tenantID, uuid, reason, string(k), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return scanEntries(rs)
}

// ---------------------------------------------------------------- Entrevistas

type InterviewRepo struct{ *Store }

func (r InterviewRepo) Search(ctx context.Context, m ports.Meta, tenantID uint64, f ports.InterviewFilter) (*domain.Page[domain.InterviewSummary], error) {
	rs, err := r.call(ctx, "sp_interview_search", m.ActorID, tenantID, nullStr(f.Search), nullStr(f.StatusID), nullStr(f.StageID),
		nullStr(f.TypeID), nullStr(f.ClientID), nullStr(f.Date), nullStr(f.TalentID), f.Page, f.PageSize)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &domain.Page[domain.InterviewSummary]{Items: []domain.InterviewSummary{}}
	rs.Next()
	if err := rs.One(&out.Total); err != nil {
		return nil, err
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var s domain.InterviewSummary
			var second, req sql.NullString
			var typ, stage, status, client []byte
			var d time.Time
			var tm string
			if err := scan(&s.ID, &s.TalentID, &s.FirstName, &s.LastName, &second, &s.TypeCode, &typ, &s.StageCode, &stage,
				&s.StatusCode, &status, &client, &req, &s.Profile, &d, &tm, &s.RatingGeneral); err != nil {
				return err
			}
			s.SecondLastName, s.RequirementRef = strPtr(second), strPtr(req)
			s.Type, s.Stage, s.Status = json.RawMessage(typ), json.RawMessage(stage), json.RawMessage(status)
			if len(client) > 0 {
				s.Client = json.RawMessage(client)
			}
			s.Date, s.Time = d.Format("2006-01-02"), hhmm(tm)
			out.Items = append(out.Items, s)
			return nil
		})
	}
	if err != nil {
		return nil, err
	}
	return out, rs.Close()
}

// hhmm recorta "15:04:05" a "15:04".
func hhmm(s string) string {
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func (r InterviewRepo) Get(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) (*ports.InterviewRecord, error) {
	rs, err := r.call(ctx, "sp_interview_get", m.ActorID, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	rec := &ports.InterviewRecord{}
	iv := &rec.Interview
	iv.Interviewers, iv.Recordings, iv.Answers, iv.Files = []domain.Interviewer{}, []domain.Recording{}, []domain.InterviewAnswer{}, []domain.TalentFile{}
	var second, headline, durID, cliID, req, link, loc, addr, np, ne, nl, nd, cancel sql.NullString
	var durMin sql.NullInt64
	var typ, stage, status, client []byte
	var d time.Time
	var tm string
	rs.Next()
	if err := rs.One(&iv.ID, &iv.TalentID, &iv.FirstName, &iv.LastName, &second, &headline,
		&iv.TypeID, &iv.TypeCode, &typ, &iv.StageID, &iv.StageCode, &stage, &iv.StatusID, &iv.StatusCode, &status,
		&durID, &durMin, &cliID, &client, &req, &iv.Profile, &d, &tm, &link, &loc, &addr,
		&iv.RatingGeneral, &iv.RatingPersonal, &iv.RatingExperience, &iv.RatingLanguages, &iv.RatingEducation,
		&np, &ne, &nl, &nd, &cancel, &iv.Version, &iv.CreatedAt, &iv.UpdatedAt, &iv.TalentBlacklisted); err != nil {
		return nil, err
	}
	iv.SecondLastName, iv.Headline, iv.DurationID, iv.ClientID, iv.RequirementRef = strPtr(second), strPtr(headline), strPtr(durID), strPtr(cliID), strPtr(req)
	iv.MeetingLink, iv.LocationURL, iv.Address = strPtr(link), strPtr(loc), strPtr(addr)
	iv.NotesPersonal, iv.NotesExperience, iv.NotesLanguages, iv.NotesEducation, iv.CancelReason = strPtr(np), strPtr(ne), strPtr(nl), strPtr(nd), strPtr(cancel)
	if durMin.Valid {
		v := int(durMin.Int64)
		iv.DurationMinutes = &v
	}
	iv.Type, iv.Stage, iv.Status = json.RawMessage(typ), json.RawMessage(stage), json.RawMessage(status)
	if len(client) > 0 {
		iv.Client = json.RawMessage(client)
	}
	iv.Date, iv.Time = d.Format("2006-01-02"), hhmm(tm)

	each := func(fn func(scan func(...any) error) error) error {
		if !rs.Next() {
			return nil
		}
		return rs.Each(fn)
	}
	err = firstErr(
		each(func(scan func(...any) error) error {
			var x domain.Interviewer
			var enc []byte
			err := scan(&x.Name, &enc, &x.Notify)
			iv.Interviewers = append(iv.Interviewers, x)
			rec.InterviewerEmailEnc = append(rec.InterviewerEmailEnc, enc)
			return err
		}),
		each(func(scan func(...any) error) error {
			var x domain.Recording
			var d time.Time
			err := scan(&x.URL, &d)
			x.Date = d.Format("2006-01-02")
			iv.Recordings = append(iv.Recordings, x)
			return err
		}),
		each(func(scan func(...any) error) error {
			var a domain.InterviewAnswer
			var q []byte
			var ans sql.NullString
			var sort int
			err := scan(&a.QuestionID, &q, &ans, &a.Active, &sort)
			a.Question, a.Answer = json.RawMessage(q), ans.String
			iv.Answers = append(iv.Answers, a)
			return err
		}),
		each(func(scan func(...any) error) error {
			var f domain.TalentFile
			err := scan(&f.ID, &f.Category, &f.OriginalName, &f.MimeType, &f.Size, &f.CreatedAt)
			iv.Files = append(iv.Files, f)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return rec, rs.Close()
}

func (r InterviewRepo) Save(ctx context.Context, m ports.Meta, tenantID uint64, w ports.InterviewWrite) (*ports.InterviewWriteResult, error) {
	ivs, _ := json.Marshal(w.Interviewers)
	if w.Recordings == nil {
		w.Recordings = []domain.Recording{}
	}
	recs, _ := json.Marshal(w.Recordings)
	var ev any
	if w.ExpectedVersion != nil {
		ev = *w.ExpectedVersion
	}
	rs, err := r.call(ctx, "sp_interview_save", m.ActorID, tenantID, w.UUID, w.TalentID, w.TypeID, w.StageID, w.StatusID,
		optStr(w.DurationID), optStr(w.ClientID), optStr(w.RequirementRef), w.Profile, w.Date, w.Time,
		optStr(w.MeetingLink), optStr(w.LocationURL), optStr(w.Address),
		w.RGeneral, w.RPersonal, w.RExperience, w.RLanguages, w.REducation,
		optStr(w.NPersonal), optStr(w.NExperience), optStr(w.NLanguages), optStr(w.NEducation), optStr(w.CancelReason),
		string(ivs), string(recs), w.AckBlacklist, ev, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out ports.InterviewWriteResult
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&out.ID, &out.Version, &out.Created, &out.TypeCode)
		}); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r InterviewRepo) SaveAnswers(ctx context.Context, m ports.Meta, tenantID uint64, uuid string, answers []ports.AnswerWrite) error {
	if answers == nil {
		answers = []ports.AnswerWrite{}
	}
	a, _ := json.Marshal(answers)
	return r.exec(ctx, "sp_interview_answers_save", m.ActorID, tenantID, uuid, string(a))
}

func (r InterviewRepo) CreateFile(ctx context.Context, m ports.Meta, tenantID uint64, interviewUUID, typeUUID string, f domain.FileObject) (*domain.TalentFile, error) {
	rs, err := r.call(ctx, "sp_interview_file_create", m.ActorID, tenantID, interviewUUID, f.UUID, typeUUID, f.StorageKey,
		f.OriginalName, f.MimeType, f.Size, f.SHA256, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out domain.TalentFile
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&out.ID, &out.Category, &out.OriginalName, &out.MimeType, &out.Size, &out.CreatedAt)
		}); err != nil {
			return nil, err
		}
	}
	return &out, rs.Close()
}

func (r InterviewRepo) GetFile(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) (*domain.FileObject, error) {
	rs, err := r.call(ctx, "sp_interview_file_get", m.ActorID, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var f domain.FileObject
	var tid sql.NullInt64
	rs.Next()
	if err := rs.One(&f.ID, &f.UUID, &tid, &f.OwnerType, &f.Category, &f.StorageKey, &f.OriginalName, &f.MimeType,
		&f.Size, &f.SHA256, &f.Encrypted); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r InterviewRepo) DeleteFile(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) error {
	return r.exec(ctx, "sp_interview_file_delete", m.ActorID, tenantID, uuid, nullStr(m.IP))
}
