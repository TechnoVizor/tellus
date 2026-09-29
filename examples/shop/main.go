// Command shop is a small storefront backend with a Tellus admin panel. It is
// the example that grows into the public live demo.
package main

import (
	"context"
	"crypto/rand"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/TechnoVizor/tellus"
	"github.com/TechnoVizor/tellus/dashboard"
	"github.com/TechnoVizor/tellus/form"
	"github.com/TechnoVizor/tellus/table"
)

type Category struct {
	ID   uint
	Name string
}

type Product struct {
	ID            uint
	Name          string `gorm:"not null"`
	Description   string
	Price         int
	Active        bool
	Photo         string
	AvailableFrom time.Time
	// CategoryID is a pointer: the relation is optional (some demo products
	// have no category), and only a pointer stores NULL, which satisfies the
	// foreign key constraint AutoMigrate creates for Category below. A plain
	// uint would store 0 instead, and no category has id 0.
	CategoryID *uint
	Category   Category
	CreatedAt  time.Time
}

func main() {
	db, err := gorm.Open(postgres.Open(env("DATABASE_URL", "postgres://tellus:tellus@localhost:55432/tellus_dev?sslmode=disable")), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := db.AutoMigrate(&Category{}, &Product{}); err != nil {
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
				table.Image("Photo"),
				table.Text("Name").Searchable().Sortable(),
				table.Money("Price").Sortable(),
				table.Boolean("Active"),
				table.Date("AvailableFrom").Sortable(),
			).
			Form(
				form.Image("Photo"),
				form.Text("Name").Required().MaxLength(100),
				form.Textarea("Description"),
				form.Money("Price").Required(),
				form.Toggle("Active"),
				form.Date("AvailableFrom").Required(),
				form.Select("CategoryID").Label("Category").Relation("Category", "Name"),
			),
		tellus.Resource[Category](db).
			Table(
				table.Text("Name").Searchable().Sortable(),
			).
			Form(
				form.Text("Name").Required().MaxLength(100),
			),
	); err != nil {
		log.Fatal(err)
	}

	if err := panel.Dashboard(
		dashboard.Stat("Products").Icon(boxIcon).Href(panel.Prefix()+"/products").
			Value(func(ctx context.Context) (string, error) { return countOf(ctx, db, &Product{}) }),
		dashboard.Stat("Categories").Icon(tagIcon).Href(panel.Prefix()+"/categories").
			Value(func(ctx context.Context) (string, error) { return countOf(ctx, db, &Category{}) }),
		dashboard.Stat("Active products").Icon(boltIcon).
			Value(func(ctx context.Context) (string, error) {
				var n int64
				err := db.WithContext(ctx).Model(&Product{}).Where("active = ?", true).Count(&n).Error
				return strconv.FormatInt(n, 10), err
			}).
			Trend(func(ctx context.Context) (float64, bool, error) {
				var active, total int64
				if err := db.WithContext(ctx).Model(&Product{}).Where("active = ?", true).Count(&active).Error; err != nil {
					return 0, false, err
				}
				if err := db.WithContext(ctx).Model(&Product{}).Count(&total).Error; err != nil {
					return 0, false, err
				}
				if total == 0 {
					return 0, true, nil
				}
				return float64(active) / float64(total) * 100, active*2 >= total, nil
			}),
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
// instead of "Product 01", "Product 02"... Price here is whole dollars for
// readability; seedProducts converts it to the cents Product.Price stores.
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

var demoCategories = []string{"Peripherals", "Audio", "Displays & Connectivity", "Desk Setup"}

// productCategory maps a demo product to its category by name. Wireless
// Charging Pad and Smart Plug are deliberately left out, so the demo also
// shows an optional relation with nothing selected.
var productCategory = map[string]string{
	"Wireless Mouse":              "Peripherals",
	"Mechanical Keyboard":         "Peripherals",
	"Mechanical Numpad":           "Peripherals",
	"Webcam, 1080p":               "Peripherals",
	"USB Microphone":              "Peripherals",
	"Noise-Cancelling Headphones": "Audio",
	"Bluetooth Speaker":           "Audio",
	"27-inch 4K Monitor":          "Displays & Connectivity",
	"USB-C Hub, 7-in-1":           "Displays & Connectivity",
	"HDMI Cable, 2m":              "Displays & Connectivity",
	"Portable SSD, 1TB":           "Displays & Connectivity",
	"Laptop Stand":                "Desk Setup",
	"Desk Lamp with USB Charging": "Desk Setup",
	"Cable Organizer Box":         "Desk Setup",
	"Standing Desk Converter":     "Desk Setup",
	"Ergonomic Mouse Pad":         "Desk Setup",
	"Desk Mat, Large":             "Desk Setup",
	"Phone Stand, Adjustable":     "Desk Setup",
}

func seedProducts(db *gorm.DB) {
	var n int64
	db.Model(&Product{}).Count(&n)
	if n > 0 {
		return
	}
	categoryIDs := seedCategories(db)
	for i, p := range demoProducts {
		// Spreads across roughly six months so the demo shows both already
		// available and upcoming products.
		availableFrom := time.Now().AddDate(0, 0, -60+i*7)
		var categoryID *uint
		if name, ok := productCategory[p.Name]; ok {
			id := categoryIDs[name]
			categoryID = &id
		}
		db.Create(&Product{
			Name:          p.Name,
			Description:   p.Description,
			Price:         p.Price * 100,
			Active:        p.Active,
			AvailableFrom: availableFrom,
			CategoryID:    categoryID,
		})
	}
}

// seedCategories creates the demo categories on first run and returns each
// one's id by name.
func seedCategories(db *gorm.DB) map[string]uint {
	ids := make(map[string]uint, len(demoCategories))
	for _, name := range demoCategories {
		cat := Category{Name: name}
		db.Create(&cat)
		ids[name] = cat.ID
	}
	return ids
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

// countOf is the dashboard Stat widgets' shared "how many rows" value function.
func countOf(ctx context.Context, db *gorm.DB, model any) (string, error) {
	var n int64
	err := db.WithContext(ctx).Model(model).Count(&n).Error
	return strconv.FormatInt(n, 10), err
}

// Icons are inline so the example has no static-file dependency; a real
// host would more likely load these from its own asset pipeline.
const (
	boxIcon  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 8l-9-5-9 5 9 5 9-5z"/><path d="M3 8v8l9 5 9-5V8"/><path d="M12 13v8"/></svg>`
	tagIcon  = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20.59 13.41L11 3.83A2 2 0 0 0 9.59 3.24L3 3v6.59a2 2 0 0 0 .59 1.41l9.58 9.58a2 2 0 0 0 2.82 0l4.6-4.6a2 2 0 0 0 0-2.82z"/><circle cx="7.5" cy="7.5" r="1.5"/></svg>`
	boltIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2 3 14h7v8l10-12h-7z"/></svg>`
)
