package mysql

import (
	"database/sql"
	"strings"
	"testing"
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
