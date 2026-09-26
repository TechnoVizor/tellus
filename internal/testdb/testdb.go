// Package testdb gives integration tests an isolated Postgres schema.
package testdb

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open returns a GORM handle bound to a fresh schema that is dropped when the
// test ends. The test is skipped when TELLUS_TEST_DSN is not set. The DSN must
// be in URL form, for example
// postgres://tellus:tellus@localhost:55432/tellus_test?sslmode=disable
func Open(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TELLUS_TEST_DSN")
	if dsn == "" {
		t.Skip("TELLUS_TEST_DSN not set; run `docker compose up -d --wait` and export it")
	}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

	admin := connect(t, dsn, cfg)
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(buf)
	if err := admin.Exec(`CREATE SCHEMA ` + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db := connect(t, u.String(), cfg)

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`).Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// connect opens the database, retrying a few times. On some Windows machines a
// loopback connect occasionally fails with a transient connectex timeout, and a
// quick retry gets past it without hiding a database that is really down.
func connect(t testing.TB, dsn string, cfg *gorm.Config) *gorm.DB {
	t.Helper()
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		db, err := gorm.Open(postgres.Open(dsn), cfg)
		if err == nil {
			return db
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("connect: %v", lastErr)
	return nil
}
