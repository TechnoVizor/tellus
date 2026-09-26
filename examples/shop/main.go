// Command shop is a small storefront backend with a Tellus admin panel. It is
// the example that grows into the public live demo.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
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

func seedProducts(db *gorm.DB) {
	var n int64
	db.Model(&Product{}).Count(&n)
	if n > 0 {
		return
	}
	for i := 1; i <= 45; i++ {
		db.Create(&Product{
			Name:        fmt.Sprintf("Product %02d", i),
			Description: "Sample product for the demo.",
			Price:       i * 10,
			Active:      i%3 != 0,
		})
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
