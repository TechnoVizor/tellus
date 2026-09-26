package form

import "testing"

func TestTextRejectsTextPostgresCannotStore(t *testing.T) {
	for _, f := range []*TextField{Text("Name"), Textarea("Body")} {
		for _, raw := range []string{"a\x00b", "a\xffb", "\xc3"} {
			if _, msg := f.Parse(raw, tString); msg == "" {
				t.Errorf("%q must be rejected with a validation message", raw)
			}
		}
		if v, msg := f.Parse("héllo\nwörld", tString); msg != "" || v != "héllo\nwörld" {
			t.Errorf("valid multi-byte text was rejected or changed: %v %q", v, msg)
		}
	}
}
