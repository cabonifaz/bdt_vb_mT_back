// Package db expone los scripts SQL embebidos en el binario.
// V*__*.sql: migraciones versionadas (se aplican una sola vez, en orden).
// R__*.sql: migraciones repetibles (procedimientos; se reaplican si cambian).
package db

import "embed"

//go:embed migrations/*.sql procedures/*.sql
var FS embed.FS
