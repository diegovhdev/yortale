// Package dbtest prepara una base de datos para las pruebas de integración.
package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New crea un esquema temporal con db/schema.sql y devuelve un pool que trabaja dentro de él.
// El esquema se borra al terminar el test. Se salta si no hay TEST_DATABASE_URL.
//
// TEST_DATABASE_URL debe ser una conexión directa (en Neon, el host sin "-pooler"): el pooler
// reparte las sentencias entre conexiones y el SET search_path no se mantendría.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL no está definida")
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("borrar el esquema %s: %v", schema, err)
		}
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Neon ignora search_path como parámetro de conexión, así que se fija en cada conexión nueva.
	// public sigue en la ruta porque ahí están las extensiones (citext, pg_trgm).
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema+", public")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	ddl, err := os.ReadFile(schemaPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("aplicar schema.sql: %v", err)
	}
	return pool
}

// CreateUser inserta un usuario mínimo (contraseña inservible) y devuelve su id.
func CreateUser(t *testing.T, pool *pgxpool.Pool, username string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		username, username+"@example.com").Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// schemaPath es db/schema.sql, relativo a este archivo y no al paquete que se prueba.
func schemaPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "schema.sql")
}
