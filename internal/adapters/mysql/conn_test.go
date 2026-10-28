package mysql

import (
	"database/sql"
	"strings"
	"testing"

	driver "github.com/go-sql-driver/mysql"
)

// keepWarm termina cuando se cierra el pool; database/sql no exporta ese error, se reconoce por el mensaje.
func TestClosedPoolMessage(t *testing.T) {
	db, err := sql.Open("mysql", "u:p@tcp(127.0.0.1:1)/x")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := db.Conn(t.Context()); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("mensaje inesperado: %v", err)
	}
}

// Solo los interbloqueos (1213) y las esperas de bloqueo agotadas (1205) se reintentan.
func TestIsDeadlock(t *testing.T) {
	for n, want := range map[uint16]bool{1213: true, 1205: true, 1644: false, 1062: false} {
		if got := isDeadlock(&driver.MySQLError{Number: n}); got != want {
			t.Errorf("%d: %v", n, got)
		}
	}
	if isDeadlock(nil) {
		t.Error("nil no es interbloqueo")
	}
}
