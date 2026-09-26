package tellus

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"

	"github.com/TechnoVizor/tellus/internal/assets"
	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
	"github.com/TechnoVizor/tellus/internal/ui"
)

const (
	sessionCookie  = "tellus_session"
	maxBodyBytes   = 1 << 20 // form posts above 1 MiB are rejected
	loginAttempts  = 10
	loginWindow    = 10 * time.Minute
	defaultPrefix  = "/admin"
	defaultName    = "Tellus"
	defaultSession = 12 * time.Hour
)

var prefixPattern = regexp.MustCompile(`^/[A-Za-z0-9._~-]+(/[A-Za-z0-9._~-]+)*$`)

// Config configures a Panel.
type Config struct {
	// Authenticator signs users in. Use NewGormAuthenticator for the built-in
	// accounts table or implement the interface for your own users. Required.
	Authenticator Authenticator
	// SessionSecret signs session cookies. At least 32 random bytes. Required.
	SessionSecret []byte
	// Prefix is the URL path the panel is mounted at. Default "/admin". It can
	// be any path, for example a secret one.
	Prefix string
	// Name is the brand name shown in the UI. Default "Tellus".
	Name string
	// SessionTTL is how long a sign-in lasts. Default 12 hours.
	SessionTTL time.Duration
	// InsecureCookies drops the Secure cookie attribute so the panel works over
	// plain http. For local development only.
	InsecureCookies bool
}

// Panel is the admin panel. Create it with New, add resources with Register,
// then mount Handler on your router at Prefix.
type Panel struct {
	cfg     Config
	prefix  string
	signer  *secure.Signer
	csrf    secure.CSRF
	limiter *secure.Limiter
	now     func() time.Time

	mu       sync.Mutex
	built    bool
	handler  http.Handler
	nav      []navEntry
	slugs    map[string]bool
	mounters []func(*http.ServeMux)
}

type navEntry struct{ slug, label string }

// New validates cfg and returns a Panel.
func New(cfg Config) (*Panel, error) {
	if cfg.Authenticator == nil {
		return nil, errors.New("tellus: Config.Authenticator is required, use NewGormAuthenticator for the built-in accounts")
	}
	signer, err := secure.NewSigner(cfg.SessionSecret)
	if err != nil {
		return nil, fmt.Errorf("tellus: Config.SessionSecret: %w", err)
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if !prefixPattern.MatchString(prefix) || slices.Contains(strings.Split(prefix, "/"), "..") || slices.Contains(strings.Split(prefix, "/"), ".") {
		return nil, fmt.Errorf("tellus: Config.Prefix %q is not a valid URL path", cfg.Prefix)
	}
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSession
	}
	return &Panel{
		cfg:     cfg,
		prefix:  prefix,
		signer:  signer,
		csrf:    secure.CSRF{Path: prefix, Secure: !cfg.InsecureCookies},
		limiter: secure.NewLimiter(loginAttempts, loginWindow),
		now:     time.Now,
		slugs:   map[string]bool{},
	}, nil
}

// Prefix is the URL path the panel must be mounted at, without a trailing slash.
func (p *Panel) Prefix() string { return p.prefix }

// Registrable is implemented by resource builders. Pass them to Panel.Register.
type Registrable interface {
	register(p *Panel) error
}

// Register adds resources. Call it before Handler.
func (p *Panel) Register(resources ...Registrable) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.built {
		return errors.New("tellus: Register must be called before Handler")
	}
	for _, r := range resources {
		if err := r.register(p); err != nil {
			return err
		}
	}
	return nil
}

// Handler returns the panel's http.Handler. Mount it at Prefix()+"/", or call
// Mount for a *http.ServeMux. It strips the prefix itself.
func (p *Panel) Handler() http.Handler {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.built {
		p.handler = p.build()
		p.built = true
	}
	return p.handler
}

// Mount registers the panel on mux at its prefix.
func (p *Panel) Mount(mux *http.ServeMux) { mux.Handle(p.prefix+"/", p.Handler()) }

func (p *Panel) build() http.Handler {
	app := http.NewServeMux()
	app.HandleFunc("GET /login", p.loginForm)
	app.HandleFunc("POST /login", p.loginSubmit)
	app.HandleFunc("POST /logout", p.logout)
	app.HandleFunc("GET /{$}", p.authed(p.home))
	for _, mount := range p.mounters {
		mount(app)
	}

	guarded := securityHeaders(limitBody(p.csrf.Middleware(app)))
	outer := http.NewServeMux()
	outer.Handle("GET /assets/", http.StripPrefix("/assets/", assets.Handler()))
	outer.Handle("/", guarded)
	return http.StripPrefix(p.prefix, outer)
}

// url builds a panel URL from a path such as "/login".
func (p *Panel) url(path string) string { return p.prefix + path }

// --- middleware ---

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// --- sessions ---

func (p *Panel) cookiePath() string { return p.prefix }

func (p *Panel) startSession(w http.ResponseWriter, u User) {
	now := p.now()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    p.signer.Sign(u.UserID(), p.cfg.SessionTTL, now),
		Path:     p.cookiePath(),
		Expires:  now.Add(p.cfg.SessionTTL),
		HttpOnly: true,
		Secure:   !p.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (p *Panel) endSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     p.cookiePath(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !p.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// currentUser resolves the session cookie to a user. ErrUserNotFound means
// nobody is signed in.
func (p *Panel) currentUser(r *http.Request) (User, error) {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, ErrUserNotFound
	}
	id, ok := p.signer.Verify(ck.Value, p.now())
	if !ok {
		return nil, ErrUserNotFound
	}
	return p.cfg.Authenticator.UserByID(r.Context(), id)
}

type authedHandler func(w http.ResponseWriter, r *http.Request, u User)

// authed wraps a handler so it only runs for a signed-in user allowed into the
// panel.
func (p *Panel) authed(h authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := p.currentUser(r)
		switch {
		case errors.Is(err, ErrUserNotFound):
			p.redirectToLogin(w, r)
			return
		case err != nil:
			p.serverError(w, err)
			return
		case !u.CanAccessPanel():
			http.Error(w, i18n.T("login.forbidden"), http.StatusForbidden)
			return
		}
		h(w, r, u)
	}
}

func (p *Panel) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	login := p.url("/login")
	if r.Header.Get("HX-Request") == "true" {
		// A plain redirect would make htmx swap the login page into the list
		// region, so tell htmx to navigate the whole page instead.
		w.Header().Set("HX-Redirect", login)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, login, http.StatusSeeOther)
}

// --- rendering ---

func (p *Panel) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		p.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (p *Panel) serverError(w http.ResponseWriter, err error) {
	slog.Error("tellus: request failed", "error", err)
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

func (p *Panel) shell(r *http.Request, u User, title, activeSlug string) ui.Shell {
	nav := make([]ui.NavItem, 0, len(p.nav))
	for _, e := range p.nav {
		nav = append(nav, ui.NavItem{Label: e.label, Href: p.url("/" + e.slug), Active: e.slug == activeSlug})
	}
	return ui.Shell{
		Title:    title,
		Brand:    p.cfg.Name,
		Prefix:   p.prefix,
		Nav:      nav,
		UserName: u.DisplayName(),
		CSRF:     secure.CSRFToken(r.Context()),
	}
}

func (p *Panel) renderMessage(w http.ResponseWriter, r *http.Request, u User, status int, heading, message string) {
	p.render(w, r, status, ui.MessagePage(p.shell(r, u, heading, ""), heading, message))
}

// --- pages ---

func (p *Panel) home(w http.ResponseWriter, r *http.Request, u User) {
	if len(p.nav) > 0 {
		http.Redirect(w, r, p.url("/"+p.nav[0].slug), http.StatusSeeOther)
		return
	}
	p.renderMessage(w, r, u, http.StatusOK, p.cfg.Name, i18n.T("ui.no_resources"))
}
