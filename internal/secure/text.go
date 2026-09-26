package secure

import (
	"strings"
	"unicode/utf8"
)

// ValidText reports whether s can be stored in a Postgres text column: valid
// UTF-8 without NUL bytes. Postgres rejects anything else with an error.
func ValidText(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

// CleanText drops NUL bytes and invalid UTF-8 from s.
func CleanText(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
}
