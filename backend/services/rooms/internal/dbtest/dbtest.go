// Package dbtest prepara bases de datos PostgreSQL aisladas para los tests de integración del
// servicio rooms.
package dbtest

import (
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"spd.web/services/rooms/internal/store"
)

// Open conecta con la base de TEST_DATABASE_URL (una URL postgres://...) dentro de un esquema
// propio, vacío y ya migrado, que se elimina al terminar el test: los tests no comparten datos ni
// dejan restos. Sin TEST_DATABASE_URL, o con -short, el test se omite.
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if testing.Short() || dsn == "" {
		t.Skip("test de integración con PostgreSQL: define TEST_DATABASE_URL para ejecutarlo")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		t.Fatalf("TEST_DATABASE_URL debe ser una URL postgres://...: %q", dsn)
	}

	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("conectar a TEST_DATABASE_URL: %v", err)
	}
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}

	// search_path hace que todas las conexiones del pool trabajen en el esquema del test.
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := store.Open(u.String(), 4, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close(db)
		_ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error
		_ = store.Close(admin)
	})
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
