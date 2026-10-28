package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type TalentRepo struct{ *Store }

func optStr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func optBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func optBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func dateArg(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func fmtDate(t sql.NullTime) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.Format("2006-01-02")
	return &s
}

func (r TalentRepo) Search(ctx context.Context, m ports.Meta, tenantID uint64, f ports.TalentFilter) (*domain.Page[domain.TalentSummary], error) {
	var minYears any
	if f.MinYears != nil {
		minYears = *f.MinYears
	}
	rs, err := r.call(ctx, "sp_talent_search", m.ActorID, tenantID, nullStr(f.Search), nullStr(f.Skills), nullStr(f.EnglishLevel),
		minYears, nullStr(f.Position), nullStr(f.Education), nullStr(f.GradeID), nullStr(f.CollectionID), nullStr(f.Status),
		f.Page, f.PageSize)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &domain.Page[domain.TalentSummary]{Items: []domain.TalentSummary{}}
	rs.Next()
	if err := rs.One(&out.Total); err != nil {
		return nil, err
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var t domain.TalentSummary
			var second, headline, city, photo, english sql.NullString
			var country, skills []byte
			var rating sql.NullFloat64
			if err := scan(&t.ID, &t.FirstName, &t.LastName, &second, &headline, &country, &city, &rating, &photo,
				&t.YearsExperience, &t.IsFavorite, &skills, &english, &t.Version, &t.UpdatedAt, &t.IsBlacklisted); err != nil {
				return err
			}
			t.SecondLastName, t.Headline, t.City, t.PhotoID, t.EnglishLevel = strPtr(second), strPtr(headline), strPtr(city), strPtr(photo), strPtr(english)
			t.AvgRating = fPtr(rating)
			if len(country) > 0 {
				t.Country = json.RawMessage(country)
			}
			t.Skills = []domain.SkillRef{}
			if len(skills) > 0 {
				_ = json.Unmarshal(skills, &t.Skills)
			}
			out.Items = append(out.Items, t)
			return nil
		})
	}
	if err != nil {
		return nil, err
	}
	return out, rs.Close()
}

func (r TalentRepo) Get(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) (*ports.TalentRecord, error) {
	rs, err := r.call(ctx, "sp_talent_get", m.ActorID, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	rec := &ports.TalentRecord{}
	t := &rec.Talent
	t.TechSkills, t.SoftSkills, t.Experiences, t.Educations = []domain.TalentTechSkill{}, []domain.TalentSoftSkill{}, []domain.TalentExperience{}, []domain.TalentEducation{}
	t.Languages, t.Feedback, t.Files, t.Collections = []domain.TalentLanguage{}, []domain.TalentFeedback{}, []domain.TalentFile{}, []string{}

	rs.Next()
	var second, headline, docType, countryID, city, originID, originCity, linkedin, github, summary, availability, photo, cv sql.NullString
	var country, origin []byte
	var hasEq sql.NullBool
	var rating sql.NullFloat64
	if err := rs.One(&t.ID, &t.FirstName, &t.LastName, &second, &headline, &docType, &rec.DocEnc, &rec.EmailEnc, &rec.PhoneEnc, &rec.BirthEnc,
		&countryID, &country, &city, &originID, &origin, &originCity, &linkedin, &github, &summary, &availability, &hasEq, &rec.RatesEnc, &t.CanSeeRates,
		&photo, &cv, &rating, &t.Status, &t.Source, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.SecondLastName, t.Headline, t.DocTypeID, t.CountryID, t.City = strPtr(second), strPtr(headline), strPtr(docType), strPtr(countryID), strPtr(city)
	t.LinkedinURL, t.GithubURL, t.Summary, t.AvailabilityID = strPtr(linkedin), strPtr(github), strPtr(summary), strPtr(availability)
	t.PhotoID, t.CvID, t.AvgRating = strPtr(photo), strPtr(cv), fPtr(rating)
	if len(country) > 0 {
		t.Country = json.RawMessage(country)
	}
	t.OriginCountryID, t.OriginCity = strPtr(originID), strPtr(originCity)
	if len(origin) > 0 {
		t.OriginCountry = json.RawMessage(origin)
	}
	if hasEq.Valid {
		t.HasEquipment = &hasEq.Bool
	}

	each := func(fn func(scan func(...any) error) error) error {
		if !rs.Next() {
			return nil
		}
		return rs.Each(fn)
	}
	err = firstErr(
		each(func(scan func(...any) error) error {
			var s domain.TalentTechSkill
			var label []byte
			err := scan(&s.ID, &s.SkillID, &label, &s.Years)
			s.Label = json.RawMessage(label)
			t.TechSkills = append(t.TechSkills, s)
			return err
		}),
		each(func(scan func(...any) error) error {
			var s domain.TalentSoftSkill
			var label []byte
			err := scan(&s.ID, &s.SkillID, &label)
			s.Label = json.RawMessage(label)
			t.SoftSkills = append(t.SoftSkills, s)
			return err
		}),
		each(func(scan func(...any) error) error {
			var e domain.TalentExperience
			var start time.Time
			var end sql.NullTime
			var fn sql.NullString
			err := scan(&e.ID, &e.Company, &e.Position, &start, &end, &e.IsCurrent, &fn, &e.Months)
			e.StartDate, e.EndDate, e.Functions = start.Format("2006-01-02"), fmtDate(end), strPtr(fn)
			t.Experiences = append(t.Experiences, e)
			return err
		}),
		each(func(scan func(...any) error) error {
			var d domain.TalentEducation
			var gradeID sql.NullString
			var grade []byte
			var start, end sql.NullTime
			err := scan(&d.ID, &d.Institution, &d.Career, &gradeID, &grade, &start, &end, &d.IsCurrent, &d.DatePrecision)
			d.GradeID, d.StartDate, d.EndDate = strPtr(gradeID), fmtDate(start), fmtDate(end)
			if len(grade) > 0 {
				d.Grade = json.RawMessage(grade)
			}
			t.Educations = append(t.Educations, d)
			return err
		}),
		each(func(scan func(...any) error) error {
			var l domain.TalentLanguage
			var lang, level []byte
			err := scan(&l.ID, &l.LanguageID, &lang, &l.LevelID, &level, &l.LevelCode, &l.Stars)
			l.Language, l.Level = json.RawMessage(lang), json.RawMessage(level)
			t.Languages = append(t.Languages, l)
			return err
		}),
		each(func(scan func(...any) error) error {
			var f domain.TalentFeedback
			var nameEnc []byte
			err := scan(&f.ID, &f.Stars, &f.Comment, &f.AuthorID, &nameEnc, &f.CreatedAt, &f.IsMine, &f.CanDelete)
			t.Feedback = append(t.Feedback, f)
			rec.FeedbackAuthorEnc = append(rec.FeedbackAuthorEnc, nameEnc)
			return err
		}),
		each(func(scan func(...any) error) error {
			var f domain.TalentFile
			err := scan(&f.ID, &f.Category, &f.OriginalName, &f.MimeType, &f.Size, &f.CreatedAt)
			t.Files = append(t.Files, f)
			return err
		}),
		each(func(scan func(...any) error) error {
			var c string
			err := scan(&c)
			t.Collections = append(t.Collections, c)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	return rec, rs.Close()
}

func (r TalentRepo) Upsert(ctx context.Context, m ports.Meta, tenantID uint64, w ports.TalentWrite) (*ports.TalentWriteResult, error) {
	var ev any
	if w.ExpectedVersion != nil {
		ev = *w.ExpectedVersion
	}
	rs, err := r.call(ctx, "sp_talent_upsert", m.ActorID, tenantID, w.UUID, w.FirstName, w.LastName, optStr(w.SecondLastName),
		optStr(w.Headline), optStr(w.DocTypeID), optBytes(w.DocEnc), optStr(w.DocBidx), optBytes(w.EmailEnc), optStr(w.EmailBidx),
		optBytes(w.PhoneEnc), optBytes(w.BirthEnc), optStr(w.CountryID), optStr(w.City), optStr(w.OriginCountryID), optStr(w.OriginCity), optStr(w.Linkedin), optStr(w.Github), optStr(w.Summary),
		optStr(w.AvailabilityID), optBool(w.HasEquipment), optBytes(w.RatesEnc), w.SetRates, ev, nullStr(w.Source), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out ports.TalentWriteResult
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error { found = true; return scan(&out.ID, &out.Version, &out.Created) }); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r TalentRepo) SetStatus(ctx context.Context, m ports.Meta, tenantID uint64, uuid, status string) error {
	return r.exec(ctx, "sp_talent_set_status", m.ActorID, tenantID, uuid, status, nullStr(m.IP))
}

func (r TalentRepo) SetAvailability(ctx context.Context, m ports.Meta, tenantID uint64, uuid string, availabilityID *string) error {
	return r.exec(ctx, "sp_talent_set_availability", m.ActorID, tenantID, uuid, optStr(availabilityID), nullStr(m.IP))
}

func (r TalentRepo) SetEquipment(ctx context.Context, m ports.Meta, tenantID uint64, uuid string, hasEquipment *bool) error {
	return r.exec(ctx, "sp_talent_set_equipment", m.ActorID, tenantID, uuid, optBool(hasEquipment), nullStr(m.IP))
}

func (r TalentRepo) EnsureSkill(ctx context.Context, m ports.Meta, tenantID uint64, table, uuid, label string) (*domain.SkillRef, error) {
	rs, err := r.call(ctx, "sp_skill_ensure", m.ActorID, tenantID, table, uuid, label)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var s domain.SkillRef
	var lbl []byte
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error { found = true; return scan(&s.ID, &lbl) }); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	s.Label = json.RawMessage(lbl)
	return &s, nil
}

func (r TalentRepo) UpsertTechSkill(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID, uuid, skillUUID string, years float64) error {
	return r.exec(ctx, "sp_talent_tech_skill_upsert", m.ActorID, tenantID, talentUUID, uuid, skillUUID, years)
}

func (r TalentRepo) AddSoftSkill(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID, uuid, skillUUID string) error {
	return r.exec(ctx, "sp_talent_soft_skill_add", m.ActorID, tenantID, talentUUID, uuid, skillUUID)
}

func (r TalentRepo) DeleteSection(ctx context.Context, m ports.Meta, tenantID uint64, section, uuid string) error {
	return r.exec(ctx, "sp_talent_section_delete", m.ActorID, tenantID, section, uuid, nullStr(m.IP))
}

func (r TalentRepo) UpsertExperience(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID string, e domain.TalentExperience) error {
	return r.exec(ctx, "sp_talent_experience_upsert", m.ActorID, tenantID, talentUUID, e.ID, e.Company, e.Position,
		e.StartDate, dateArg(e.EndDate), e.IsCurrent, optStr(e.Functions))
}

func (r TalentRepo) UpsertEducation(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID string, e domain.TalentEducation) error {
	return r.exec(ctx, "sp_talent_education_upsert", m.ActorID, tenantID, talentUUID, e.ID, e.Institution, e.Career,
		optStr(e.GradeID), dateArg(e.StartDate), dateArg(e.EndDate), e.IsCurrent, e.DatePrecision)
}

func (r TalentRepo) UpsertLanguage(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID, uuid, languageUUID, levelUUID string, stars int) error {
	return r.exec(ctx, "sp_talent_language_upsert", m.ActorID, tenantID, talentUUID, uuid, languageUUID, levelUUID, stars)
}

func (r TalentRepo) UpsertFeedback(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID, uuid string, stars int, comment string) (*float64, error) {
	rs, err := r.call(ctx, "sp_talent_feedback_upsert", m.ActorID, tenantID, talentUUID, uuid, stars, comment)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var avg sql.NullFloat64
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error { return scan(&avg) }); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	return fPtr(avg), nil
}

func (r TalentRepo) DeleteFeedback(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) error {
	return r.exec(ctx, "sp_talent_feedback_delete", m.ActorID, tenantID, uuid, nullStr(m.IP))
}

func (r TalentRepo) CreateFile(ctx context.Context, m ports.Meta, tenantID uint64, talentUUID string, f domain.FileObject) (*domain.TalentFile, error) {
	rs, err := r.call(ctx, "sp_talent_file_create", m.ActorID, tenantID, talentUUID, f.UUID, f.Category, f.StorageKey,
		f.OriginalName, f.MimeType, f.Size, f.SHA256, f.Encrypted, nullStr(m.IP))
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
	if err := rs.Close(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r TalentRepo) GetFile(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) (*domain.FileObject, error) {
	rs, err := r.call(ctx, "sp_talent_file_get", m.ActorID, tenantID, uuid)
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

func (r TalentRepo) DeleteFile(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) error {
	return r.exec(ctx, "sp_talent_file_delete", m.ActorID, tenantID, uuid, nullStr(m.IP))
}

func scanCollections(rs *resultSets) ([]ports.CollectionRecord, error) {
	out := []ports.CollectionRecord{}
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			var c ports.CollectionRecord
			err := scan(&c.ID, &c.Name, &c.TalentCount, &c.Access, &c.OwnerID, &c.OwnerNameEnc, &c.ShareCount, &c.UpdatedAt)
			out = append(out, c)
			return err
		}); err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}

func scanPeople(rs *resultSets, withPerm bool) ([]ports.PersonRecord, error) {
	out := []ports.PersonRecord{}
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			var p ports.PersonRecord
			var err error
			if withPerm {
				err = scan(&p.ID, &p.NameEnc, &p.EmailEnc, &p.Permission, &p.CreatedAt)
			} else {
				err = scan(&p.ID, &p.NameEnc, &p.EmailEnc)
			}
			out = append(out, p)
			return err
		}); err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}

func (r TalentRepo) Collections(ctx context.Context, m ports.Meta, tenantID uint64) ([]ports.CollectionRecord, error) {
	rs, err := r.call(ctx, "sp_collection_list", m.ActorID, tenantID)
	if err != nil {
		return nil, err
	}
	return scanCollections(rs)
}

func (r TalentRepo) CreateCollection(ctx context.Context, m ports.Meta, tenantID uint64, uuid, name string) ([]ports.CollectionRecord, error) {
	rs, err := r.call(ctx, "sp_collection_create", m.ActorID, tenantID, uuid, name)
	if err != nil {
		return nil, err
	}
	return scanCollections(rs)
}

func (r TalentRepo) RenameCollection(ctx context.Context, m ports.Meta, tenantID uint64, uuid, name string) error {
	return r.exec(ctx, "sp_collection_rename", m.ActorID, tenantID, uuid, name)
}

func (r TalentRepo) DeleteCollection(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) error {
	return r.exec(ctx, "sp_collection_delete", m.ActorID, tenantID, uuid)
}

func (r TalentRepo) SetCollectionTalent(ctx context.Context, m ports.Meta, tenantID uint64, collectionUUID, talentUUID string, in bool) error {
	return r.exec(ctx, "sp_collection_set_talent", m.ActorID, tenantID, collectionUUID, talentUUID, in)
}

func (r TalentRepo) CollectionShares(ctx context.Context, m ports.Meta, tenantID uint64, uuid string) ([]ports.PersonRecord, error) {
	rs, err := r.call(ctx, "sp_collection_shares", m.ActorID, tenantID, uuid)
	if err != nil {
		return nil, err
	}
	return scanPeople(rs, true)
}

func (r TalentRepo) ShareCollection(ctx context.Context, m ports.Meta, tenantID uint64, uuid, userUUID, permission string) ([]ports.PersonRecord, error) {
	rs, err := r.call(ctx, "sp_collection_share", m.ActorID, tenantID, uuid, userUUID, permission, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return scanPeople(rs, true)
}

func (r TalentRepo) UnshareCollection(ctx context.Context, m ports.Meta, tenantID uint64, uuid, userUUID string) error {
	return r.exec(ctx, "sp_collection_unshare", m.ActorID, tenantID, uuid, userUUID, nullStr(m.IP))
}

func (r TalentRepo) Colleagues(ctx context.Context, m ports.Meta, tenantID uint64) ([]ports.PersonRecord, error) {
	rs, err := r.call(ctx, "sp_tenant_colleagues", m.ActorID, tenantID)
	if err != nil {
		return nil, err
	}
	return scanPeople(rs, false)
}

// ---------------- Sincronización offline ----------------

type SyncRepo struct{ *Store }

func (r SyncRepo) Register(ctx context.Context, tenantID, userID uint64, opID, entity, op string) (bool, string, []byte, error) {
	rs, err := r.call(ctx, "sp_sync_register", tenantID, userID, opID, entity, op, nil, time.Now().UTC())
	if err != nil {
		return false, "", nil, err
	}
	defer rs.Close()
	var dup bool
	var status string
	var result []byte
	rs.Next()
	if err := rs.One(&dup, &status, &result); err != nil {
		return false, "", nil, err
	}
	return dup, status, result, rs.Close()
}

func (r SyncRepo) Complete(ctx context.Context, tenantID uint64, opID, status string, result []byte) error {
	var res any
	if len(result) > 0 {
		res = string(result) // JSON: enviar como texto utf8, no binario
	}
	return r.exec(ctx, "sp_sync_complete", tenantID, opID, status, res)
}

func (r TalentRepo) Match(ctx context.Context, m ports.Meta, tenantID uint64, emailBidx, docBidx, first, last, second *string) (*domain.TalentMatch, error) {
	rs, err := r.call(ctx, "sp_talent_match", m.ActorID, tenantID, emailBidx, docBidx, first, last, second)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out *domain.TalentMatch
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var x domain.TalentMatch
			var second, headline sql.NullString
			if err := scan(&x.ID, &x.FirstName, &x.LastName, &second, &headline, &x.MatchedBy); err != nil {
				return err
			}
			x.SecondLastName, x.Headline = strPtr(second), strPtr(headline)
			out = &x
			return nil
		})
	}
	return out, err
}
