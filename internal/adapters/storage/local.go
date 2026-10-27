// Package storage guarda archivos en disco local. En Railway STORAGE_PATH apunta a un
// volumen persistente montado (p. ej. /data/files), así sobreviven a cada despliegue.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrTooLarge = errors.New("archivo excede el tamaño máximo")

type Local struct{ root string }

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

// resolve impide path traversal: la clave siempre queda dentro de root.
func (l *Local) resolve(key string) (string, error) {
	clean := filepath.Clean("/" + strings.ReplaceAll(key, "\\", "/"))
	p := filepath.Join(l.root, filepath.FromSlash(clean))
	if !strings.HasPrefix(p, l.root+string(os.PathSeparator)) {
		return "", errors.New("clave de almacenamiento inválida")
	}
	return p, nil
}

func (l *Local) Put(_ context.Context, key string, r io.Reader, maxBytes int64) (int64, string, error) {
	p, err := l.resolve(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, "", err
	}
	tmp := p + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, maxBytes+1))
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err == nil && n > maxBytes {
		err = ErrTooLarge
	}
	if err != nil {
		os.Remove(tmp)
		return 0, "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.resolve(key)
	if err != nil {
		return err
	}
	return os.Remove(p)
}
