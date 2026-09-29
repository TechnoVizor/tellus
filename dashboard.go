package tellus

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"gorm.io/gorm"
)

// dashboardCard is what one registered resource contributes to the
// dashboard home page. Built once at Register time (resource.go), the same
// split relation.go's optionLoaders already use: register-time reflection
// with the resource's own concrete type, request-time data access through a
// plain, non-generic closure.
type dashboardCard struct {
	label    string
	href     string
	countFn  func(ctx context.Context) (int64, error)
	seriesFn func(ctx context.Context, days int) ([]dailyCount, error) // nil when unavailable
}

// dailyCount is one day's row count, as resolveTimeSeries's query returns
// it: sparse, only days with at least one row.
type dailyCount struct {
	Day   time.Time
	Count int64
}

// resolveTimeSeries returns nil when db is nil (a custom Source with no
// *gorm.DB, the same GORM-only stance relation.go already takes for
// relations) or when model has no CreatedAt field of type time.Time with a
// real column. Otherwise it returns a closure that groups the model's rows
// by day.
//
// ponytail: date_trunc is Postgres-specific, the same tradeoff
// gormsource.go's List already makes for its ILIKE search (docs/spec.md
// section 5, "Postgres first"). Use a portable bucketing expression when
// another database is supported.
func resolveTimeSeries(db *gorm.DB, model reflect.Type) func(ctx context.Context, days int) ([]dailyCount, error) {
	if db == nil {
		return nil
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(reflect.New(model).Interface()); err != nil {
		return nil
	}
	sf, ok := stmt.Schema.FieldsByName["CreatedAt"]
	if !ok || sf.FieldType != reflect.TypeOf(time.Time{}) || sf.DBName == "" {
		return nil
	}
	table, quotedCol := stmt.Schema.Table, db.Statement.Quote(sf.DBName)
	return func(ctx context.Context, days int) ([]dailyCount, error) {
		since := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
		var rows []struct {
			Day   time.Time
			Count int64
		}
		// quotedCol and table are schema names resolved once above, never
		// request input, so building the query from them is safe, the same
		// argument relation.go's loader already makes. The "day" and "count"
		// aliases, and the Group/Order calls below, are hardcoded literals
		// this function chose itself, never a model's own field or column
		// name, so unlike relation.go's label column they carry no
		// reserved-word risk.
		err := db.WithContext(ctx).Table(table).
			Select("date_trunc('day', "+quotedCol+") AS day, COUNT(*) AS count").
			Where(quotedCol+" >= ?", since).
			Group("day").Order("day").
			Find(&rows).Error
		if err != nil {
			return nil, err
		}
		out := make([]dailyCount, len(rows))
		for i, r := range rows {
			out[i] = dailyCount{Day: r.Day, Count: r.Count}
		}
		return out, nil
	}
}

// sparklineJSON fills any day with no rows to zero, so the chart's x-axis
// spacing is even, then encodes uPlot's own data shape: a two-element array
// of a timestamps array and a values array, x as Unix seconds.
func sparklineJSON(rows []dailyCount, days int) string {
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Day.Format("2006-01-02")] = r.Count
	}
	start := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	xs := make([]int64, days)
	ys := make([]int64, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		xs[i] = day.Unix()
		ys[i] = counts[day.Format("2006-01-02")]
	}
	b, _ := json.Marshal([2][]int64{xs, ys})
	return string(b)
}
