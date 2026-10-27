package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strings"
)

// Migrate aplica las migraciones versionadas (V) y repetibles (R) del FS.
func Migrate(ctx context.Context, db *sql.DB, fsys fs.FS, log *slog.Logger) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migration (
		script     VARCHAR(200) PRIMARY KEY,
		kind       CHAR(1)      NOT NULL,
		checksum   CHAR(64)     NOT NULL,
		applied_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
	) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("schema_migration: %w", err)
	}

	applied := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT script, checksum FROM schema_migration`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var s, c string
		if err := rows.Scan(&s, &c); err != nil {
			rows.Close()
			return err
		}
		applied[s] = c
	}
	rows.Close()

	for _, dir := range []string{"migrations", "procedures"} {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".sql") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			kind := name[:1]
			raw, err := fs.ReadFile(fsys, path.Join(dir, name))
			if err != nil {
				return err
			}
			// checksum sobre contenido normalizado (LF) para que CRLF de Windows no cambie la huella
			sum := sha256.Sum256([]byte(strings.ReplaceAll(string(raw), "\r\n", "\n")))
			checksum := hex.EncodeToString(sum[:])
			prev, done := applied[name]
			switch {
			case done && prev == checksum:
				continue
			case done && kind == "V":
				return fmt.Errorf("la migración %s ya aplicada fue modificada; crea una nueva versión", name)
			}
			log.Info("aplicando script", "script", name)
			for i, stmt := range SplitSQL(string(raw)) {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("%s (sentencia %d): %w\n%s", name, i+1, err, preview(stmt))
				}
			}
			if _, err := db.ExecContext(ctx, `INSERT INTO schema_migration (script, kind, checksum) VALUES (?, ?, ?)
				ON DUPLICATE KEY UPDATE checksum = VALUES(checksum), applied_at = CURRENT_TIMESTAMP(3)`, name, kind, checksum); err != nil {
				return err
			}
		}
	}
	return nil
}

// SplitSQL separa un script en sentencias respetando directivas DELIMITER
// (mismo comportamiento que el cliente mysql). Una sentencia termina cuando
// una línea acaba con el delimitador vigente.
func SplitSQL(script string) []string {
	delim := ";"
	var out []string
	var cur strings.Builder
	flush := func() {
		s := strings.TrimSpace(cur.String())
		cur.Reset()
		if s != "" && !onlyComments(s) {
			out = append(out, s)
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "DELIMITER ") {
			flush()
			delim = strings.TrimSpace(trimmed[len("DELIMITER "):])
			continue
		}
		if strings.HasSuffix(trimmed, delim) {
			cur.WriteString(strings.TrimSuffix(trimmed, delim))
			flush()
			continue
		}
		cur.WriteString(line)
		cur.WriteString("\n")
	}
	flush()
	return out
}

func onlyComments(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "--") {
			return false
		}
	}
	return true
}

func preview(s string) string {
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
