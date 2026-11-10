package app

import (
	"context"
	"encoding/json"
	"io"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// API de integración (key con alcance talent.read; la usa IGDT con la llave BDT automática): ficha completa, CV y
// postulaciones de los talentos que son candidatos en algún RQ del tenant de la key. Sin tarifas. Cada lectura se audita
// con la key.

// IntegrationDossier: ficha del candidato con los datos personales descifrados.
func (s *TalentService) IntegrationDossier(ctx context.Context, c *domain.ApiClient, ip, talentID string) (json.RawMessage, error) {
	if c == nil {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	doc, err := s.Repo.IntegrationDossier(ctx, c.TenantID, c.KeyID, talentID, ip)
	if err != nil {
		return nil, err
	}
	return decryptKeysOpt(s.Cipher, doc, true, "docNumberEnc", "emailEnc", "phoneEnc", "birthDateEnc", "authorNameEnc"), nil
}

// IntegrationFile: CV del candidato (original o formato empresa ES/EN), descifrado.
func (s *TalentService) IntegrationFile(ctx context.Context, c *domain.ApiClient, ip, talentID, fileID string) (*domain.FileObject, []byte, error) {
	if c == nil {
		return nil, nil, domain.NewError("APIKEY_INVALID")
	}
	if !validUUID(talentID) || !validUUID(fileID) {
		return nil, nil, domain.NewError("FILE_NOT_FOUND")
	}
	f, err := s.Repo.IntegrationFile(ctx, c.TenantID, c.KeyID, talentID, fileID, ip)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, domain.NewError("FILE_NOT_FOUND")
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxTalentFile*2))
	if err != nil {
		return nil, nil, err
	}
	if f.Encrypted {
		if raw, err = s.Cipher.DecryptBytes(raw); err != nil {
			return nil, nil, err
		}
	}
	return f, raw, nil
}
