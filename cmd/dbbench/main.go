// Mide la latencia hacia MySQL: viaje de red, consultas con parámetros (preparadas vs interpoladas),
// conexión nueva y tiempo de servidor de los SP más usados. Uso: go run ./cmd/dbbench
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
)

func open(c config.Config, interpolate bool, compress ...bool) *sql.DB {
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = c.DBUser, c.DBPassword, "tcp", c.DBHost+":"+c.DBPort, c.DBName
	cfg.ParseTime, cfg.InterpolateParams = true, interpolate
	if len(compress) > 0 && compress[0] {
		_ = cfg.Apply(driver.EnableCompression(true))
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(2)
	return db
}

func avg(n int, f func()) time.Duration {
	f() // calentar
	t := time.Now()
	for i := 0; i < n; i++ {
		f()
	}
	return time.Since(t) / time.Duration(n)
}

func main() {
	config.LoadDotEnv(".env")
	c, err := config.Load()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	ctx := context.Background()
	prep, interp := open(c, false), open(c, true)
	comp := open(c, true, true)
	var x any
	fmt.Println("SELECT 1 (1 viaje)            :", avg(5, func() { _ = interp.QueryRowContext(ctx, "SELECT 1").Scan(&x) }))
	fmt.Println("SELECT ? preparado (2 viajes) :", avg(5, func() { _ = prep.QueryRowContext(ctx, "SELECT ?", 1).Scan(&x) }))
	fmt.Println("SELECT ? interpolado (1 viaje):", avg(5, func() { _ = interp.QueryRowContext(ctx, "SELECT ?", 1).Scan(&x) }))
	fmt.Println("conexión nueva + SELECT 1     :", avg(3, func() {
		d := open(c, true)
		_ = d.QueryRowContext(ctx, "SELECT 1").Scan(&x)
		d.Close()
	}))
	// tiempo de servidor de los SP: medido dentro de MySQL (sin la red)
	var tenant, actor int64
	_ = interp.QueryRowContext(ctx, "SELECT t.id, u.id FROM tenant t JOIN app_user u ON u.tenant_id = t.id JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id AND r.code = 'TENANT_ADMIN' WHERE t.slug = 'fractal' AND u.status = 'ACTIVE' LIMIT 1").Scan(&tenant, &actor)
	for _, db := range []*sql.DB{interp, comp, interp, comp} {
		for _, q := range []string{
			fmt.Sprintf("CALL sp_talent_search(%d, %d, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 1, 25)", actor, tenant),
			fmt.Sprintf("CALL sp_rq_board(%d, %d, NULL, NULL, 0, 60)", actor, tenant),
			fmt.Sprintf("CALL sp_notification_list(%d, %d, 0, NULL, 30)", actor, tenant),
			fmt.Sprintf("CALL sp_home_summary(%d, %d, CURDATE(), NOW())", actor, tenant),
		} {
			t := time.Now()
			rows, err := db.QueryContext(ctx, q)
			if err != nil {
				fmt.Println("ERR", q[:40], err)
				continue
			}
			n := 0
			for {
				for rows.Next() {
					n++
				}
				if !rows.NextResultSet() {
					break
				}
			}
			rows.Close()
			fmt.Printf("%-6s %-36s total %v (%d filas)\n", map[bool]string{true: "zlib", false: "plano"}[db == comp], q[5:36], time.Since(t), n)
		}
	}
}
