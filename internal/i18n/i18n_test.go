package i18n

import "testing"

func TestTKnownAndUnknownKeys(t *testing.T) {
	if got := T("common.yes"); got != "Yes" {
		t.Errorf("known key: got %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key should echo the key, got %q", got)
	}
}

func TestCatalogHasNoEmptyValues(t *testing.T) {
	for k, v := range en {
		if v == "" {
			t.Errorf("key %q has an empty value", k)
		}
	}
}
