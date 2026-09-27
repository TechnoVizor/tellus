package tellus

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus/internal/testdb"
)

type Product struct {
	ID        uint
	Name      string
	Price     int
	Active    bool
	Photo     string // an image URL, empty when the product has none
	CreatedAt time.Time
}

type Note struct {
	ID        uint
	Body      string
	DeletedAt gorm.DeletedAt
}

func productSource(t *testing.T, n int) (*GormSource[Product], *gorm.DB) {
	t.Helper()
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		p := Product{Name: fmt.Sprintf("Product %02d", i), Price: i * 10, Active: i%2 == 0}
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	src, err := NewGormSource[Product](db)
	if err != nil {
		t.Fatal(err)
	}
	return src, db
}

func TestGormSourcePagination(t *testing.T) {
	src, _ := productSource(t, 25)
	ctx := context.Background()

	res, err := src.List(ctx, ListQuery{Page: 1, PerPage: 10})
	if err != nil || len(res.Items) != 10 || res.Total != 25 {
		t.Fatalf("page 1: len=%d total=%d err=%v", len(res.Items), res.Total, err)
	}
	res, _ = src.List(ctx, ListQuery{Page: 3, PerPage: 10})
	if len(res.Items) != 5 || res.Items[0].Name != "Product 21" {
		t.Fatalf("page 3: %+v", res.Items)
	}
	res, _ = src.List(ctx, ListQuery{Page: 9, PerPage: 10})
	if len(res.Items) != 0 || res.Total != 25 {
		t.Fatalf("page past the end: len=%d total=%d", len(res.Items), res.Total)
	}
	res, _ = src.List(ctx, ListQuery{})
	if len(res.Items) != 25 {
		t.Fatalf("zero-value query must use defaults, got %d items", len(res.Items))
	}
}

func TestGormSourceSort(t *testing.T) {
	src, _ := productSource(t, 5)
	ctx := context.Background()

	res, err := src.List(ctx, ListQuery{SortField: "Price", SortDesc: true, PerPage: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.Items[0].Price != 50 || res.Items[4].Price != 10 {
		t.Fatalf("descending sort wrong: %+v", res.Items)
	}
	res, _ = src.List(ctx, ListQuery{SortField: "Name", PerPage: 5})
	if res.Items[0].Name != "Product 01" {
		t.Fatalf("ascending sort wrong: %+v", res.Items)
	}
}

func TestGormSourceSearch(t *testing.T) {
	src, db := productSource(t, 12)
	ctx := context.Background()
	db.Create(&Product{Name: "100% pure"})
	db.Create(&Product{Name: "1000 pure"})
	db.Create(&Product{Name: "a_b"})
	db.Create(&Product{Name: "axb"})

	search := func(q string, fields ...string) []Product {
		t.Helper()
		res, err := src.List(ctx, ListQuery{Search: q, SearchFields: fields, PerPage: 50})
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		return res.Items
	}

	if got := search("product 07", "Name"); len(got) != 1 {
		t.Errorf("case-insensitive match: %d", len(got))
	}
	if got := search("100%", "Name"); len(got) != 1 || got[0].Name != "100% pure" {
		t.Errorf("percent must be literal: %+v", got)
	}
	if got := search("a_b", "Name"); len(got) != 1 || got[0].Name != "a_b" {
		t.Errorf("underscore must be literal: %+v", got)
	}
	if got := search("120", "Name", "Price"); len(got) != 1 || got[0].Price != 120 {
		t.Errorf("numeric column must be searchable as text: %+v", got)
	}
	if got := search(`'; DROP TABLE products; --`, "Name"); len(got) != 0 {
		t.Errorf("injection string matched rows: %+v", got)
	}
	if got := search("Product", "Name"); len(got) != 12 {
		t.Errorf("table damaged or search broken: %d rows", len(got))
	}
	if got := search("anything"); len(got) != 16 {
		t.Errorf("no search fields means no filter, got %d", len(got))
	}
}

func TestGormSourceRejectsUnknownFields(t *testing.T) {
	src, _ := productSource(t, 2)
	ctx := context.Background()
	if _, err := src.List(ctx, ListQuery{SortField: "Price; DROP TABLE products"}); err == nil {
		t.Error("unknown sort field must be an error")
	}
	if _, err := src.List(ctx, ListQuery{Search: "x", SearchFields: []string{"Nope"}}); err == nil {
		t.Error("unknown search field must be an error")
	}
}

func TestGormSourceCRUD(t *testing.T) {
	src, _ := productSource(t, 0)
	ctx := context.Background()

	p := &Product{Name: "Lamp", Price: 30, Active: true}
	if err := src.Create(ctx, p); err != nil || p.ID == 0 {
		t.Fatalf("create: id=%d err=%v", p.ID, err)
	}
	id := src.ID(p)
	if id != fmt.Sprint(p.ID) {
		t.Fatalf("ID() = %q", id)
	}

	got, err := src.Find(ctx, id)
	if err != nil || got.Name != "Lamp" {
		t.Fatalf("find: %+v %v", got, err)
	}

	got.Name, got.Price, got.Active = "Desk lamp", 0, false
	if err := src.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := src.Find(ctx, id)
	if again.Name != "Desk lamp" || again.Price != 0 || again.Active {
		t.Fatalf("update must persist zero values: %+v", again)
	}

	if err := src.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("find after delete: %v", err)
	}
	if err := src.Update(ctx, got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a deleted record must be ErrNotFound, got %v", err)
	}
	var count int64
	src.db.Model(&Product{}).Count(&count)
	if count != 0 {
		t.Fatalf("update resurrected the deleted record, count=%d", count)
	}
}

func TestGormSourceBadIDs(t *testing.T) {
	src, _ := productSource(t, 1)
	ctx := context.Background()
	for _, id := range []string{"abc", "", "-1", "1.5", "99999999999999999999"} {
		if _, err := src.Find(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Find(%q): want ErrNotFound, got %v", id, err)
		}
		if err := src.Delete(ctx, id); err != nil {
			t.Errorf("Delete(%q) of a missing record must not fail: %v", id, err)
		}
	}
}

func TestGormSourceSoftDelete(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Note{}); err != nil {
		t.Fatal(err)
	}
	src, err := NewGormSource[Note](db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	n := &Note{Body: "hello"}
	if err := src.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	if err := src.Delete(ctx, src.ID(n)); err != nil {
		t.Fatal(err)
	}
	res, _ := src.List(ctx, ListQuery{})
	if res.Total != 0 {
		t.Fatalf("soft-deleted record still listed: %+v", res)
	}
	if _, err := src.Find(ctx, src.ID(n)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("soft-deleted record still found: %v", err)
	}
}

func TestNewGormSourceRejectsUnsupportedModels(t *testing.T) {
	db := testdb.Open(t)
	type NoKey struct{ Name string }
	type Composite struct {
		A uint `gorm:"primaryKey"`
		B uint `gorm:"primaryKey"`
	}
	type FloatKey struct {
		ID float64 `gorm:"primaryKey"`
	}
	if _, err := NewGormSource[NoKey](db); err == nil {
		t.Error("model without a primary key must be rejected")
	}
	if _, err := NewGormSource[Composite](db); err == nil {
		t.Error("composite primary key must be rejected")
	}
	if _, err := NewGormSource[FloatKey](db); err == nil {
		t.Error("float primary key must be rejected")
	}
	if _, err := NewGormSource[Product](nil); err == nil {
		t.Error("nil db must be rejected")
	}
}
