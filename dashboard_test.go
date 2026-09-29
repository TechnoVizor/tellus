package tellus

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/testdb"
	"github.com/TechnoVizor/tellus/table"
)

// dashActivity and dashNoTimestamp are fixtures dedicated to dashboard
// tests: one with the CreatedAt convention resolveTimeSeries looks for, one
// without, kept separate from the shared Product/Note/relation fixtures.
type dashActivity struct {
	ID        uint
	Name      string
	CreatedAt time.Time
}

type dashNoTimestamp struct {
	ID   uint
	Name string
}

func TestResolveTimeSeriesGroupsByDayAndExcludesOutsideTheWindow(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	create := func(daysAgo int, n int) {
		for i := 0; i < n; i++ {
			if err := db.Create(&dashActivity{Name: "x", CreatedAt: now.AddDate(0, 0, -daysAgo)}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	create(0, 2)  // today
	create(5, 3)  // 5 days ago
	create(40, 1) // outside the 30-day window, must be excluded

	loader := resolveTimeSeries(db, reflect.TypeOf(dashActivity{}))
	if loader == nil {
		t.Fatal("expected a series loader for a model with CreatedAt")
	}
	rows, err := loader(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, r := range rows {
		total += r.Count
	}
	if total != 5 {
		t.Errorf("total in window = %d, want 5 (the row 40 days ago must be excluded)", total)
	}
	found := false
	for _, r := range rows {
		if r.Day.Format("2006-01-02") == now.AddDate(0, 0, -5).Format("2006-01-02") && r.Count == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("missing the 3-row day 5 days ago: %+v", rows)
	}
}

func TestResolveTimeSeriesEmptyWindow(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	// Only a row outside the window: the query must return no error and no
	// rows, not fail, and sparklineJSON on that empty result must still
	// produce a full zero-filled series (checked in the sibling test below).
	if err := db.Create(&dashActivity{Name: "old", CreatedAt: time.Now().AddDate(0, 0, -90)}).Error; err != nil {
		t.Fatal(err)
	}
	loader := resolveTimeSeries(db, reflect.TypeOf(dashActivity{}))
	rows, err := loader(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("expected no rows inside the window, got %+v", rows)
	}
	raw := sparklineJSON(rows, 30)
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded[0]) != 30 || len(decoded[1]) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(decoded[0]), len(decoded[1]))
	}
	for i, y := range decoded[1] {
		if y != 0 {
			t.Errorf("day %d should be zero, got %d", i, y)
		}
	}
}

func TestResolveTimeSeriesNilCases(t *testing.T) {
	db := testdb.Open(t)
	if resolveTimeSeries(nil, reflect.TypeOf(dashActivity{})) != nil {
		t.Error("nil db must yield a nil series loader")
	}
	if resolveTimeSeries(db, reflect.TypeOf(dashNoTimestamp{})) != nil {
		t.Error("a model with no CreatedAt field must yield a nil series loader")
	}
}

func TestSparklineJSONFillsGapsAndKeepsRealCounts(t *testing.T) {
	today := time.Now().Truncate(24 * time.Hour)
	rows := []dailyCount{
		{Day: today.AddDate(0, 0, -29), Count: 7},
		{Day: today, Count: 3},
	}
	raw := sparklineJSON(rows, 30)
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	xs, ys := decoded[0], decoded[1]
	if len(xs) != 30 || len(ys) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(xs), len(ys))
	}
	if ys[0] != 7 {
		t.Errorf("first day count = %d, want 7", ys[0])
	}
	if ys[29] != 3 {
		t.Errorf("last day (today) count = %d, want 3", ys[29])
	}
	for i := 1; i < 29; i++ {
		if ys[i] != 0 {
			t.Errorf("day %d should be zero-filled, got %d", i, ys[i])
		}
	}
}

func TestDashboardPageRendersRealSeries(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	today := time.Now()
	db.Create(&dashActivity{Name: "a", CreatedAt: today})
	db.Create(&dashActivity{Name: "b", CreatedAt: today})
	db.Create(&dashActivity{Name: "c", CreatedAt: today.AddDate(0, 0, -10)})

	res := Resource[dashActivity](db).Slug("activity").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required())
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	result := h.get("/")
	if result.status != http.StatusOK {
		t.Fatalf("status %d", result.status)
	}
	body := result.body
	i := strings.Index(body, `data-series="`)
	if i < 0 {
		t.Fatal("missing data-series attribute")
	}
	rest := body[i+len(`data-series="`):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("unterminated data-series attribute")
	}
	var decoded [2][]int64
	if err := json.Unmarshal([]byte(rest[:j]), &decoded); err != nil {
		t.Fatalf("decode %q: %v", rest[:j], err)
	}
	xs, ys := decoded[0], decoded[1]
	if len(xs) != 30 || len(ys) != 30 {
		t.Fatalf("lengths = %d, %d, want 30, 30", len(xs), len(ys))
	}
	if ys[29] != 2 {
		t.Errorf("today's count (index 29) = %d, want 2", ys[29])
	}
	if ys[19] != 1 {
		t.Errorf("10-days-ago count (index 19 = 29-10) = %d, want 1", ys[19])
	}
}

func TestHomeSeriesErrorIsServerError(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	res := Resource[dashActivity](db).Slug("activity").
		Table(table.Text("Name")).
		Form(form.Text("Name").Required())
	h := newHarness(t, Config{}, res)
	h.loginAdmin()

	if err := db.Migrator().DropTable(&dashActivity{}); err != nil {
		t.Fatal(err)
	}
	result := h.get("/")
	if result.status != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", result.status)
	}
	if !strings.Contains(result.body, "Something went wrong on our side") {
		t.Errorf("expected the generic server-error message, got: %s", result.body)
	}
}
