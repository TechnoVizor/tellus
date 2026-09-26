package secure

import "testing"

func TestValidText(t *testing.T) {
	for _, s := range []string{"", "hello", "héllo", "日本語", "tab\tand\nnewline"} {
		if !ValidText(s) {
			t.Errorf("ValidText(%q) = false, want true", s)
		}
	}
	// Postgres text columns reject NUL bytes and invalid UTF-8.
	for _, s := range []string{"a\x00b", "\x00", "\xff", "a\xffb", "\xc3"} {
		if ValidText(s) {
			t.Errorf("ValidText(%q) = true, want false", s)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := CleanText("a\x00b\xffc"); got != "abc" {
		t.Errorf("CleanText dropped the wrong bytes: %q", got)
	}
	if got := CleanText("héllo\nwörld"); got != "héllo\nwörld" {
		t.Errorf("CleanText changed valid text: %q", got)
	}
}
