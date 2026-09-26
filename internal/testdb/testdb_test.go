package testdb

import "testing"

func TestOpenGivesIsolatedSchema(t *testing.T) {
	db := Open(t)

	var schema string
	if err := db.Raw(`SELECT current_schema()`).Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	if len(schema) < 3 || schema[:2] != "t_" {
		t.Fatalf("expected a t_* schema, got %q", schema)
	}
	if err := db.Exec(`CREATE TABLE probe (id int)`).Error; err != nil {
		t.Fatalf("table create in isolated schema failed: %v", err)
	}
}
