package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type RequirementRepo struct{ *Store }

func optF(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func optI(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func optJSON(j json.RawMessage) any {
	if len(j) == 0 || string(j) == "null" {
		return nil
	}
	return string(j)
}

// docs lee un result set de una columna JSON (doc) y, si hay, el nombre cifrado del responsable.
func (r RequirementRepo) docs(rs *resultSets, withOwner bool) ([]ports.RQDoc, error) {
	out := []ports.RQDoc{}
	err := rs.Each(func(scan func(...any) error) error {
		var d ports.RQDoc
		var doc []byte
		dest := []any{&doc}
		if withOwner {
			dest = append(dest, &d.OwnerNameEnc)
		}
		if err := scan(dest...); err != nil {
			return err
		}
		d.Doc = json.RawMessage(doc)
		out = append(out, d)
		return nil
	})
	return out, err
}

func (r RequirementRepo) rawDocs(ctx context.Context, proc string, args ...any) ([]json.RawMessage, error) {
	rs, err := r.call(ctx, proc, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []json.RawMessage{}
	if rs.Next() {
		docs, err := r.docs(rs, false)
		if err != nil {
			return nil, err
		}
		for _, d := range docs {
			out = append(out, d.Doc)
		}
	}
	return out, nil
}

func (r RequirementRepo) Board(ctx context.Context, m ports.Meta, tenantID uint64, f ports.BoardQuery) ([]ports.RQDoc, ports.BoardCounts, error) {
	var n ports.BoardCounts
	rs, err := r.call(ctx, "sp_rq_board", m.ActorID, tenantID, nullStr(f.Search), f.ClientID, f.ContactID, f.ProfileID, f.Mine, f.Scope, f.Today)
	if err != nil {
		return nil, n, err
	}
	defer rs.Close()
	if !rs.Next() {
		return []ports.RQDoc{}, n, nil
	}
	docs, err := r.docs(rs, true)
	if err != nil {
		return nil, n, err
	}
	if rs.Next() {
		if err := rs.One(&n.Active, &n.Soon, &n.Overdue, &n.New, &n.Closed, &n.All); err != nil {
			return nil, n, err
		}
	}
	return docs, n, nil
}

func (r RequirementRepo) Get(ctx context.Context, m ports.Meta, tenantID uint64, id string) (*ports.RQDoc, error) {
	rs, err := r.call(ctx, "sp_rq_get", m.ActorID, tenantID, id)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var d ports.RQDoc
	var doc []byte
	if rs.Next() {
		if err := rs.One(&doc, &d.OwnerNameEnc); err != nil {
			return nil, err
		}
	}
	d.Doc = json.RawMessage(doc)
	return &d, nil
}

func (r RequirementRepo) change(rs *resultSets, withCreated bool) (*ports.RQChange, error) {
	defer rs.Close()
	var c ports.RQChange
	var prev, client []byte
	if !rs.Next() {
		return nil, nil
	}
	var err error
	if withCreated {
		err = rs.One(&c.ID, &c.Created, &c.Version, &c.Code, &prev, &c.Status, &c.Title, &client)
	} else {
		err = rs.One(&c.ID, &c.Version, &c.Code, &prev, &c.Status, &c.Title, &client)
	}
	c.PreviousStatus, c.Client = string(prev), string(client)
	return &c, err
}

func (r RequirementRepo) Save(ctx context.Context, m ports.Meta, tenantID uint64, w ports.RQWrite) (*ports.RQChange, error) {
	rs, err := r.call(ctx, "sp_rq_save", m.ActorID, tenantID, w.ID, nullStr(w.Code), w.ClientID, w.Title, nullStr(w.Description), w.StatusID,
		w.RequestDate, w.DueDate, optF(w.DurationValue), w.DurationUnitID, optF(w.ContractValue), w.ContractUnitID, w.WorkModalityID,
		w.NeedsEmo, w.NeedsSctr, w.NeedsLaptop, w.OwnerID, optJSON(w.Contacts), optJSON(w.Billing), optJSON(w.Vacancies),
		optI(w.ExpectedVersion), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.change(rs, true)
}

func (r RequirementRepo) SetStatus(ctx context.Context, m ports.Meta, tenantID uint64, id, statusID string, order *int) (*ports.RQChange, error) {
	rs, err := r.call(ctx, "sp_rq_set_status", m.ActorID, tenantID, id, statusID, optI(order), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.change(rs, false)
}

func (r RequirementRepo) Reopen(ctx context.Context, m ports.Meta, tenantID uint64, id, reason, notes string, statusID *string) (*ports.RQChange, error) {
	rs, err := r.call(ctx, "sp_rq_reopen", m.ActorID, tenantID, id, reason, nullStr(notes), statusID, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.change(rs, false)
}

func (r RequirementRepo) Delete(ctx context.Context, m ports.Meta, tenantID uint64, id string) error {
	return r.exec(ctx, "sp_rq_delete", m.ActorID, tenantID, id, nullStr(m.IP))
}

func (r RequirementRepo) CandidateAdd(ctx context.Context, m ports.Meta, tenantID uint64, rqID, id, talentID, vacancyID string) (bool, error) {
	rs, err := r.call(ctx, "sp_rq_candidate_add", m.ActorID, tenantID, rqID, id, talentID, vacancyID, nullStr(m.IP))
	if err != nil {
		return false, err
	}
	defer rs.Close()
	var uuid string
	var created bool
	if rs.Next() {
		err = rs.One(&uuid, &created)
	}
	return created, err
}

func (r RequirementRepo) CandidateMove(ctx context.Context, m ports.Meta, tenantID uint64, id, stageID string, vacancyID *string, order *int) (string, error) {
	rs, err := r.call(ctx, "sp_rq_candidate_move", m.ActorID, tenantID, id, stageID, vacancyID, optI(order), nullStr(m.IP))
	if err != nil {
		return "", err
	}
	defer rs.Close()
	var rq, from, to string
	if rs.Next() {
		err = rs.One(&rq, &from, &to)
	}
	return to, err
}

func (r RequirementRepo) CandidateRemove(ctx context.Context, m ports.Meta, tenantID uint64, id string) error {
	return r.exec(ctx, "sp_rq_candidate_remove", m.ActorID, tenantID, id, nullStr(m.IP))
}

func (r RequirementRepo) ByTalent(ctx context.Context, m ports.Meta, tenantID uint64, talentID string) ([]json.RawMessage, error) {
	return r.rawDocs(ctx, "sp_rq_by_talent", m.ActorID, tenantID, talentID)
}

func (r RequirementRepo) Contacts(ctx context.Context, m ports.Meta, tenantID uint64, clientID string) ([]json.RawMessage, error) {
	return r.rawDocs(ctx, "sp_client_contacts", m.ActorID, tenantID, clientID)
}

func (r RequirementRepo) ContactSave(ctx context.Context, m ports.Meta, tenantID uint64, c ports.ContactWrite) error {
	return r.exec(ctx, "sp_client_contact_save", m.ActorID, tenantID, c.ID, c.ClientID, c.FirstName, c.LastName, nullStr(c.SecondLastName),
		nullStr(c.Position), nullStr(c.Phone), nullStr(c.Phone2), nullStr(c.Email), nullStr(c.Email2), nullStr(m.IP))
}

func (r RequirementRepo) ContactDelete(ctx context.Context, m ports.Meta, tenantID uint64, id string) error {
	return r.exec(ctx, "sp_client_contact_delete", m.ActorID, tenantID, id, nullStr(m.IP))
}
