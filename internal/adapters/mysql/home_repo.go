package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type HomeRepo struct{ *Store }

func (r HomeRepo) Summary(ctx context.Context, m ports.Meta, tenantID uint64, today, now time.Time) (*domain.HomeSummary, error) {
	rs, err := r.call(ctx, "sp_home_summary", m.ActorID, tenantID, today.Format("2006-01-02"), now.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &domain.HomeSummary{Interviews: []domain.HomeInterview{}, Talents: []domain.HomeTalent{}, Months: []domain.HomeMonthStats{}}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var iv domain.HomeInterview
			var second, req sql.NullString
			var stage, status, client []byte
			var d time.Time
			var tm string
			if err := scan(&iv.ID, &iv.TalentID, &iv.FirstName, &iv.LastName, &second, &iv.TypeCode, &stage, &iv.StatusCode, &status,
				&client, &req, &iv.Profile, &d, &tm, &iv.IsMine); err != nil {
				return err
			}
			iv.SecondLastName, iv.RequirementRef = strPtr(second), strPtr(req)
			iv.Stage, iv.Status = json.RawMessage(stage), json.RawMessage(status)
			if len(client) > 0 {
				iv.Client = json.RawMessage(client)
			}
			iv.Date, iv.Time = d.Format("2006-01-02"), hhmm(tm)
			out.Interviews = append(out.Interviews, iv)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var t domain.HomeTalent
			var second, headline, photo sql.NullString
			var created time.Time
			if err := scan(&t.ID, &t.FirstName, &t.LastName, &second, &headline, &photo, &created, &t.IsNew); err != nil {
				return err
			}
			t.SecondLastName, t.Headline, t.PhotoID = strPtr(second), strPtr(headline), strPtr(photo)
			t.CreatedAt = created.UTC().Format(time.RFC3339)
			out.Talents = append(out.Talents, t)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var s domain.HomeMonthStats
			if err := scan(&s.Month, &s.MyPlanned, &s.MyDone, &s.AreaPlanned, &s.AreaDone, &s.TalentsNew, &s.MyTalentsNew); err != nil {
				return err
			}
			out.Months = append(out.Months, s)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}
