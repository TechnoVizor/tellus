package humanize

import "testing"

func TestName(t *testing.T) {
	cases := map[string]string{
		"Name":        "Name",
		"name":        "Name",
		"CreatedAt":   "Created at",
		"ID":          "ID",
		"CategoryID":  "Category ID",
		"HTMLBody":    "HTML body",
		"order_items": "Order items",
		"products":    "Products",
		"Line1Text":   "Line1 text",
		"":            "",
	}
	for in, want := range cases {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}
