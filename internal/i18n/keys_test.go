package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryKeyUsedInSourceExists catches a typo in a key before it reaches the
// screen as a raw "list.emtpy".
func TestEveryKeyUsedInSourceExists(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	call := regexp.MustCompile(`i18n\.T\("([^"]+)"\)`)

	found := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".templ") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range call.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s uses unknown i18n key %q", strings.TrimPrefix(path, root), m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 20 {
		t.Fatalf("scanned the wrong tree: only %d i18n.T calls found under %s", found, root)
	}
}

// TestNoticeKeysExist covers the keys built at run time in resource_handlers.go.
func TestNoticeKeysExist(t *testing.T) {
	for _, kind := range []string{"created", "updated", "deleted"} {
		if _, ok := en["notice."+kind]; !ok {
			t.Errorf("missing notice.%s", kind)
		}
	}
}
