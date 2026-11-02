// dbq ejecuta una consulta de solo lectura (SELECT) en la base del ambiente y la imprime separada por tabuladores.
// Soporte de staging: DSN=usuario:clave@tcp(host:puerto)/base go run ./cmd/dbq "SELECT ..."
package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	if len(os.Args) < 2 || !strings.EqualFold(strings.Fields(os.Args[1])[0], "SELECT") {
		fmt.Fprintln(os.Stderr, "uso: dbq \"SELECT ...\" (solo consultas de lectura)")
		os.Exit(2)
	}
	db, err := sql.Open("mysql", os.Getenv("DSN"))
	if err != nil {
		fail(err)
	}
	defer db.Close()
	rows, err := db.Query(os.Args[1])
	if err != nil {
		fail(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	fmt.Println(strings.Join(cols, "\t"))
	for rows.Next() {
		vals := make([]sql.RawBytes, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			fail(err)
		}
		out := make([]string, len(cols))
		for i, v := range vals {
			out[i] = string(v)
		}
		fmt.Println(strings.Join(out, "\t"))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
