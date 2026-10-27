package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	driver "github.com/go-sql-driver/mysql"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Store agrupa los repositorios. Toda la persistencia pasa por procedimientos almacenados:
// este paquete nunca construye SELECT/INSERT/UPDATE, solo CALL.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// resultSets permite leer en orden los conjuntos de resultados de un CALL.
type resultSets struct {
	rows  *sql.Rows
	first bool
}

func (s *Store) call(ctx context.Context, proc string, args ...any) (*resultSets, error) {
	q := "CALL " + proc + "(" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	return &resultSets{rows: rows, first: true}, nil
}

// exec ejecuta un procedimiento descartando resultados.
func (s *Store) exec(ctx context.Context, proc string, args ...any) error {
	rs, err := s.call(ctx, proc, args...)
	if err != nil {
		return err
	}
	return rs.Close()
}

// Next avanza al siguiente result set (el primero ya está activo).
func (r *resultSets) Next() bool {
	if r.first {
		r.first = false
		return true
	}
	return r.rows.NextResultSet()
}

// Each escanea cada fila del result set actual.
func (r *resultSets) Each(fn func(scan func(dest ...any) error) error) error {
	for r.rows.Next() {
		if err := fn(r.rows.Scan); err != nil {
			return err
		}
	}
	return mapErr(r.rows.Err())
}

// One escanea la primera fila del result set actual; ErrNotFound si no hay filas.
func (r *resultSets) One(dest ...any) error {
	found := false
	err := r.Each(func(scan func(...any) error) error {
		if found {
			return nil
		}
		found = true
		return scan(dest...)
	})
	if err != nil {
		return err
	}
	if !found {
		return domain.ErrNotFound
	}
	return nil
}

func (r *resultSets) Close() error {
	// Drenar todos los result sets para que errores SIGNAL tardíos afloren.
	for r.rows.NextResultSet() {
	}
	err := r.rows.Err()
	r.rows.Close()
	return mapErr(err)
}

// mapErr traduce SIGNAL SQLSTATE '45000' 'BDT:CODIGO' (MySQL 1644) a domain.Error.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var me *driver.MySQLError
	if errors.As(err, &me) && me.Number == 1644 && strings.HasPrefix(me.Message, "BDT:") {
		return domain.NewError(strings.TrimPrefix(me.Message, "BDT:"))
	}
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullU64(p *uint64) any {
	if p == nil {
		return nil
	}
	return *p
}
