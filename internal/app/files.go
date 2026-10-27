package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type FileService struct {
	Repo    ports.FileRepository
	Storage ports.FileStorage
	Tenants ports.TenantRepository
}

// Tipos de imagen permitidos para branding (sin SVG: puede contener scripts).
var brandingMimes = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/x-icon": "ico", "image/vnd.microsoft.icon": "ico"}

const maxBrandingBytes = 2 << 20 // 2 MB

// UploadBranding guarda logo/favicon en el volumen persistente y lo asocia al branding.
func (s *FileService) UploadBranding(ctx context.Context, p *domain.Principal, ip, kind, originalName string, r io.Reader) (*domain.Branding, error) {
	if kind != "LOGO" && kind != "LOGO_DARK" && kind != "FAVICON" && kind != "CV_HEADER" && kind != "CV_FOOTER" {
		return nil, domain.NewError("INVALID_KIND")
	}
	br := bufio.NewReader(r)
	head, _ := br.Peek(512)
	mime := http.DetectContentType(head)
	if len(head) >= 4 && head[0] == 0 && head[1] == 0 && head[2] == 1 && head[3] == 0 {
		mime = "image/x-icon"
	}
	ext, ok := brandingMimes[mime]
	if !ok {
		return nil, domain.NewError("FILE_TYPE_NOT_ALLOWED")
	}
	id := uuid.NewString()
	key := path.Join("tenants", p.TenantUUID, "branding", id+"."+ext)
	size, sum, err := s.Storage.Put(ctx, key, br, maxBrandingBytes)
	if err != nil {
		if strings.Contains(err.Error(), "tamaño") {
			return nil, domain.NewError("FILE_TOO_LARGE")
		}
		return nil, err
	}
	name := path.Base(strings.ReplaceAll(originalName, "\\", "/"))
	if len(name) > 200 {
		name = name[:200]
	}
	f := domain.FileObject{UUID: id, TenantID: p.TenantID, OwnerType: "TENANT_BRANDING", Category: kind,
		StorageKey: key, OriginalName: name, MimeType: mime, Size: size, SHA256: sum}
	if err := s.Repo.Create(ctx, meta(p, ip), f, p.TenantID); err != nil {
		_ = s.Storage.Delete(ctx, key)
		return nil, err
	}
	return s.Tenants.SetBrandingFile(ctx, meta(p, ip), *p.TenantID, kind, id)
}

// OpenPublic abre solo archivos de branding (logos visibles en el login).
func (s *FileService) OpenPublic(ctx context.Context, fileUUID string) (*domain.FileObject, io.ReadCloser, error) {
	if _, err := uuid.Parse(fileUUID); err != nil {
		return nil, nil, domain.ErrNotFound
	}
	f, err := s.Repo.Get(ctx, nil, fileUUID, true)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, domain.ErrNotFound
	}
	return f, rc, nil
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
