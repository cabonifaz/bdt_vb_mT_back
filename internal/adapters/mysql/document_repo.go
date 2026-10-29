package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type DocumentRepo struct{ *Store }

func (r DocumentRepo) Rules(ctx context.Context, m ports.Meta, tenantID uint64) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_doc_rules", m.ActorID, tenantID)
}

func (r DocumentRepo) SaveRule(ctx context.Context, m ports.Meta, tenantID uint64, d ports.DocRule) error {
	return r.exec(ctx, "sp_doc_rule_save", m.ActorID, tenantID, d.TypeID, d.DefaultMode, d.Reusable, optI(d.ValidityDays),
		d.Verification, d.Active, d.SortOrder, nullStr(m.IP))
}

func (r DocumentRepo) CandidateDetail(ctx context.Context, m ports.Meta, tenantID uint64, candidateID, today string) (json.RawMessage, error) {
	rs, err := r.call(ctx, "sp_rq_candidate_detail", m.ActorID, tenantID, candidateID, today)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var doc []byte
	if rs.Next() {
		if err := rs.One(&doc); err != nil {
			return nil, err
		}
	}
	return json.RawMessage(doc), nil
}

func (r DocumentRepo) CandidateTalent(ctx context.Context, m ports.Meta, tenantID uint64, candidateID string) (string, error) {
	rs, err := r.call(ctx, "sp_candidate_talent", m.ActorID, tenantID, candidateID)
	if err != nil {
		return "", err
	}
	defer rs.Close()
	var id string
	if rs.Next() {
		err = rs.One(&id)
	}
	return id, err
}

func (r DocumentRepo) Create(ctx context.Context, m ports.Meta, tenantID uint64, id, candidateID, typeID, fileID, today string) (*ports.DocContext, error) {
	rs, err := r.call(ctx, "sp_talent_document_create", m.ActorID, tenantID, id, candidateID, typeID, fileID, today, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return docContext(rs)
}

func (r DocumentRepo) Context(ctx context.Context, m ports.Meta, tenantID uint64, id string) (*ports.DocContext, error) {
	rs, err := r.call(ctx, "sp_talent_document_get", m.ActorID, tenantID, id)
	if err != nil {
		return nil, err
	}
	return docContext(rs)
}

// docContext lee el último result set (el contexto lo devuelve un SP anidado, después de los otros).
func docContext(rs *resultSets) (*ports.DocContext, error) {
	defer rs.Close()
	var c ports.DocContext
	var validity sql.NullInt64
	var ruc, docType sql.NullString
	found := false
	for rs.Next() {
		err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&c.ID, &c.TypeCode, &c.Verification, &validity, &ruc, &c.TalentDocEnc, &docType, &c.FileID, &c.FileSHA256)
		})
		if err != nil {
			return nil, err
		}
		if found {
			break
		}
	}
	if !found {
		return nil, domain.ErrNotFound
	}
	if validity.Valid {
		v := int(validity.Int64)
		c.ValidityDays = &v
	}
	c.TenantRUC, c.TalentDocType = ruc.String, docType.String
	return &c, nil
}

func (r DocumentRepo) Verify(ctx context.Context, m ports.Meta, tenantID uint64, id, candidateID string, v ports.DocVerdict) error {
	return r.exec(ctx, "sp_talent_document_verify", m.ActorID, tenantID, id, candidateID, v.Status, v.Method, nullStr(v.Reason),
		optJSON(v.Result), optDate(v.IssuedAt), optDate(v.ExpiresAt), nullStr(m.IP))
}

func optDate(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}
