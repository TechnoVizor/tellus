// Command shop is a small storefront backend with a Tellus admin panel. It is
// the example that grows into the public live demo.
package main

import (
	"context"
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus"
	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

type Product struct {
	ID          uint
	Name        string `gorm:"not null"`
	Description string
	Price       int
	Active      bool
	CreatedAt   time.Time
}

func main() {
	db, err := gorm.Open(postgres.Open(env("DATABASE_URL", "postgres://tellus:tellus@localhost:55432/tellus_dev?sslmode=disable")), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&Product{}); err != nil {
		log.Fatal(err)
	}
	seedProducts(db)

	auth := tellus.NewGormAuthenticator(db)
	if err := auth.Migrate(); err != nil {
		log.Fatal(err)
	}
	seedAdmin(auth)

	panel, err := tellus.New(tellus.Config{
		Authenticator:   auth,
		SessionSecret:   sessionSecret(),
		Prefix:          env("ADMIN_PREFIX", "/admin"),
		Name:            "Shop admin",
		InsecureCookies: true, // plain http on localhost
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := panel.Register(
		tellus.Resource[Product](db).
			Table(
				table.Text("Name").Searchable().Sortable(),
				table.Text("Price").Sortable(),
				table.Boolean("Active"),
			).
			Form(
				form.Text("Name").Required().MaxLength(100),
				form.Textarea("Description"),
				form.Number("Price").Required(),
				form.Toggle("Active"),
			),
	); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	panel.Mount(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, panel.Prefix()+"/", http.StatusSeeOther)
	})

	addr := env("ADDR", ":8091")
	log.Printf("shop admin on http://localhost%s%s", addr, panel.Prefix())
	log.Fatal(http.ListenAndServe(addr, mux))
}

// demoProducts gives the demo a catalog that looks real in a screenshot,
// instead of "Product 01", "Product 02"... Price is a plain integer (dollars):
// Tellus does not have a Money column type yet, see docs/spec.md section 9.
var demoProducts = []struct {
	Name        string
	Description string
	Price       int
	Active      bool
}{
	{"Wireless Mouse", "Silent buttons, 2.4 GHz receiver, up to 18 months on two AA batteries.", 25, true},
	{"Mechanical Keyboard", "Hot-swappable switches, per-key RGB, aluminum top plate.", 90, true},
	{"USB-C Hub, 7-in-1", "HDMI 4K, two USB-A, SD and microSD, 100W passthrough.", 40, true},
	{"27-inch 4K Monitor", "IPS panel, 99% sRGB, height and tilt adjustable stand.", 350, true},
	{"Noise-Cancelling Headphones", "Over-ear, 30-hour battery, USB-C fast charge.", 180, true},
	{"Webcam, 1080p", "Autofocus, built-in mic, privacy shutter.", 45, false},
	{"Laptop Stand", "Aluminum, folds flat, fits 11 to 17-inch laptops.", 30, true},
	{"Desk Lamp with USB Charging", "Three color modes, touch dimmer, two USB-A ports.", 35, true},
	{"Portable SSD, 1TB", "USB 3.2 Gen 2, up to 1050 MB/s read.", 85, true},
	{"Wireless Charging Pad", "15W fast charge, works through most cases.", 20, false},
	{"Mechanical Numpad", "Same switches as the full keyboard, USB-C.", 35, true},
	{"Cable Organizer Box", "Hides power strips and cable clutter under a desk.", 15, true},
	{"Standing Desk Converter", "Sits on any desk, spring-assisted lift, two tiers.", 130, true},
	{"Ergonomic Mouse Pad", "Memory foam wrist rest, stitched edges.", 13, true},
	{"HDMI Cable, 2m", "4K at 60Hz, braided jacket.", 9, true},
	{"Bluetooth Speaker", "IPX7 waterproof, 12-hour battery, USB-C.", 60, true},
	{"Phone Stand, Adjustable", "Aluminum, folds for travel, fits most phones and tablets.", 18, true},
	{"Desk Mat, Large", "900x400mm, stitched edges, non-slip base.", 23, false},
	{"Smart Plug", "Wi-Fi, schedules and energy monitoring, no hub required.", 16, true},
	{"USB Microphone", "Cardioid condenser, plug and play, built-in stand.", 70, true},
}

func seedProducts(db *gorm.DB) {
	var n int64
	db.Model(&Product{}).Count(&n)
	if n > 0 {
		return
	}
	for _, p := range demoProducts {
		db.Create(&Product{Name: p.Name, Description: p.Description, Price: p.Price, Active: p.Active})
	}
}

// seedAdmin creates the demo account on first run. Local demo credentials only.
func seedAdmin(auth *tellus.GormAuthenticator) {
	email := env("ADMIN_EMAIL", "admin@example.com")
	password := env("ADMIN_PASSWORD", "demo-password-123")
	if _, err := auth.Authenticate(context.Background(), email, password); err == nil {
		return
	}
	if err := auth.CreateUser(context.Background(), email, "Demo admin", password); err != nil {
		log.Printf("demo admin not created (it may exist with another password): %v", err)
	}
}

// sessionSecret uses SESSION_SECRET when set. Otherwise it generates a random
// one, so every restart signs everyone out.
func sessionSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return b
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
