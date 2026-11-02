package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type VersionRepo struct{ *Store }

func (r VersionRepo) Register(ctx context.Context, version, env, status, commit string, notes json.RawMessage) (bool, error) {
	rs, err := r.call(ctx, "sp_app_version_register", version, env, status, nullStr(commit), string(notes))
	if err != nil {
		return false, err
	}
	defer rs.Close()
	var isNew bool
	if rs.Next() {
		err = rs.One(&isNew)
	}
	return isNew, err
}

func (r VersionRepo) Current(ctx context.Context, env, version string) (json.RawMessage, error) {
	docs, err := RequirementRepo(r).rawDocs(ctx, "sp_app_version_current", env, version)
	if err != nil || len(docs) == 0 {
		return nil, err
	}
	return docs[0], nil
}

func (r VersionRepo) List(ctx context.Context, env string) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_app_version_list", env)
}

func (r VersionRepo) SetStatus(ctx context.Context, m ports.Meta, env, version, status string) error {
	return r.exec(ctx, "sp_app_version_set_status", m.ActorID, env, version, status, nullStr(m.IP))
}
