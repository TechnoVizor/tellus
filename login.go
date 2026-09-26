package tellus

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/TechnoVizor/tellus/internal/i18n"
	"github.com/TechnoVizor/tellus/internal/secure"
	"github.com/TechnoVizor/tellus/internal/ui"
)

func (p *Panel) loginView(r *http.Request, email, message string) ui.LoginView {
	return ui.LoginView{
		Brand:  p.cfg.Name,
		Prefix: p.prefix,
		Action: p.url("/login"),
		Email:  email,
		Error:  message,
		CSRF:   secure.CSRFToken(r.Context()),
	}
}

func (p *Panel) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, err := p.currentUser(r); err == nil {
		http.Redirect(w, r, p.url("/"), http.StatusSeeOther)
		return
	}
	p.render(w, r, http.StatusOK, ui.LoginPage(p.loginView(r, "", "")))
}

func (p *Panel) loginSubmit(w http.ResponseWriter, r *http.Request) {
	email := strings.TrimSpace(r.PostFormValue("email"))

	key := clientIP(r)
	if !p.limiter.Allow(key) {
		p.render(w, r, http.StatusTooManyRequests, ui.LoginPage(p.loginView(r, email, i18n.T("login.too_many"))))
		return
	}

	user, err := p.cfg.Authenticator.Authenticate(r.Context(), email, r.PostFormValue("password"))
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		p.render(w, r, http.StatusUnauthorized, ui.LoginPage(p.loginView(r, email, i18n.T("login.invalid"))))
		return
	case err != nil:
		p.serverError(w, err)
		return
	case !user.CanAccessPanel():
		// Same message as a wrong password so the response does not reveal that
		// the account exists.
		p.render(w, r, http.StatusUnauthorized, ui.LoginPage(p.loginView(r, email, i18n.T("login.invalid"))))
		return
	}

	p.startSession(w, user)
	p.limiter.Reset(key) // a person behind a shared address is not locked out by their own sign-ins
	http.Redirect(w, r, p.url("/"), http.StatusSeeOther)
}

func (p *Panel) logout(w http.ResponseWriter, r *http.Request) {
	p.endSession(w)
	http.Redirect(w, r, p.url("/login"), http.StatusSeeOther)
}

// clientIP is the key the sign-in limiter counts by: the peer address, with an
// IPv6 address reduced to its /64 network, because one host or customer holds a
// whole /64 and could otherwise use a fresh address for every attempt. Behind a
// reverse proxy, set r.RemoteAddr from the trusted forwarding header (for
// example with chi's RealIP middleware) before the request reaches the panel.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	ip = ip.WithZone("").Unmap()
	if ip.Is6() {
		if prefix, err := ip.Prefix(64); err == nil {
			return prefix.Addr().String()
		}
	}
	return ip.String()
}
