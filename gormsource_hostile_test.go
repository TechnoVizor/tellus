package tellus

import (
	"context"
	"errors"
	"testing"

	"github.com/TechnoVizor/tellus/internal/testdb"
)

// Tag has a string primary key, Small a 32-bit one.
type Tag struct {
	ID    string `gorm:"primaryKey"`
	Label string
}

type Small struct {
	ID   int32 `gorm:"primaryKey"`
	Name string
}

func TestGormSourceStringKeysRejectHostileIDs(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Tag{}); err != nil {
		t.Fatal(err)
	}
	src, err := NewGormSource[Tag](db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db.Create(&Tag{ID: "ok", Label: "x"})

	// Postgres rejects NUL bytes and invalid UTF-8 in text, so such an id can
	// never exist and must not turn into a database error.
	for _, id := range []string{"a\x00b", "\xff", "a\xffb"} {
		if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Find(%q): want ErrNotFound, got %v", id, err)
		}
		if err := src.Delete(ctx, id); err != nil {
			t.Errorf("Delete(%q) of a missing record must not fail: %v", id, err)
		}
	}
	if got, err := src.Find(ctx, "ok"); err != nil || got.Label != "x" {
		t.Fatalf("ordinary id broke: %+v %v", got, err)
	}
}

func TestGormSourceNarrowIntegerKeysTreatHugeIDsAsMissing(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Small{}); err != nil {
		t.Fatal(err)
	}
	src, err := NewGormSource[Small](db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	item := &Small{Name: "a"}
	if err := src.Create(ctx, item); err != nil {
		t.Fatal(err)
	}

	// pgx refuses to send these to an int4 column, which used to become a 500.
	for _, id := range []string{"99999999999", "2147483648", "-2147483649"} {
		if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Find(%q): want ErrNotFound, got %v", id, err)
		}
		if err := src.Delete(ctx, id); err != nil {
			t.Errorf("Delete(%q) of a missing record must not fail: %v", id, err)
		}
	}
	if got, err := src.Find(ctx, src.ID(item)); err != nil || got.Name != "a" {
		t.Fatalf("ordinary id broke: %+v %v", got, err)
	}
}
