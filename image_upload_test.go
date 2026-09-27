package tellus

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/internal/testdb"
	"github.com/TechnoVizor/tellus/table"
)

func TestImageUploadEndToEnd(t *testing.T) {
	db := testdb.Open(t)
	if err := db.AutoMigrate(&Product{}); err != nil {
		t.Fatal(err)
	}
	store, err := NewLocalStorage(t.TempDir(), "/uploads")
	if err != nil {
		t.Fatal(err)
	}

	products := Resource[Product](db).
		Table(table.Text("Name"), table.Image("Photo")).
		Form(form.Text("Name").Required(), form.Image("Photo"))

	h := newHarness(t, Config{Storage: store}, products)
	h.loginAdmin()

	png := append([]byte("\x89PNG\r\n\x1a\n"), []byte(strings.Repeat("x", 50))...)
	res := h.postMultipart("/products", map[string]string{"Name": "Lamp"}, "Photo", "lamp.png", png)
	if res.status != http.StatusSeeOther {
		t.Fatalf("create: %d %s", res.status, res.body)
	}

	var p Product
	if err := db.Where("name = ?", "Lamp").First(&p).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Photo, "/uploads/") {
		t.Fatalf("photo not stored: %q", p.Photo)
	}
	data, err := os.ReadFile(filepath.Join(store.dir, strings.TrimPrefix(p.Photo, "/uploads/")))
	if err != nil || !bytes.Equal(data, png) {
		t.Fatalf("file on disk: %v", err)
	}

	list := h.get("/products")
	if !strings.Contains(list.body, `src="`+p.Photo+`"`) {
		t.Errorf("thumbnail missing from the list: %s", list.body)
	}

	// Editing without choosing a new file keeps the photo.
	id := fmt.Sprint(p.ID)
	res = h.postMultipart("/products/"+id, map[string]string{"Name": "Renamed Lamp"}, "", "", nil)
	if res.status != http.StatusSeeOther {
		t.Fatalf("update: %d %s", res.status, res.body)
	}
	var again Product
	db.First(&again, p.ID)
	if again.Photo != p.Photo {
		t.Errorf("photo changed without a new upload: %q -> %q", p.Photo, again.Photo)
	}

	// A rejected upload (wrong type) keeps the old photo and shows a message.
	res = h.postMultipart("/products/"+id, map[string]string{"Name": "Renamed Lamp"}, "Photo", "notes.txt", []byte("plain text, not an image"))
	if res.status != http.StatusUnprocessableEntity {
		t.Fatalf("rejected upload: %d", res.status)
	}
	if !strings.Contains(res.body, "JPEG, PNG, GIF or WEBP") {
		t.Errorf("missing validation message: %s", res.body)
	}
	db.First(&again, p.ID)
	if again.Photo != p.Photo {
		t.Error("a rejected upload changed the stored photo")
	}
}
