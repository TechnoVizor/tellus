package secure

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/TechnoVizor/tellus/internal/i18n"
)

// CSRF cookie, request header and form field names.
const (
	CSRFCookie = "tellus_csrf"
	CSRFHeader = "X-CSRF-Token"
	CSRFField  = "_csrf"
)

type csrfKey struct{}

// CSRF protects state-changing requests with the double-submit cookie pattern:
// the token in the cookie must be echoed in the X-CSRF-Token header or the
// _csrf form field. Forms and htmx requests get the token from CSRFToken.
//
// ponytail: the cookie is not bound to the session, so a sibling subdomain that
// can set cookies could fix a known token. Bind it to the session if that
// threat matters.
type CSRF struct {
	Path   string // cookie path, the panel prefix (empty means "/")
	Secure bool   // set the Secure cookie attribute
}

// Middleware issues the cookie on first contact, stores the token in the
// request context and rejects unsafe requests without a matching token.
func (c CSRF) Middleware(next http.Handler) http.Handler {
	path := c.Path
	if path == "" {
		path = "/"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if ck, err := r.Cookie(CSRFCookie); err == nil && validCSRFToken(ck.Value) {
			token = ck.Value
		}
		if !isSafeMethod(r.Method) {
			sent := r.Header.Get(CSRFHeader)
			if sent == "" {
				sent = r.PostFormValue(CSRFField)
			}
			if token == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				http.Error(w, i18n.T("error.csrf"), http.StatusForbidden)
				return
			}
		}
		if token == "" {
			token = newCSRFToken()
			http.SetCookie(w, &http.Cookie{
				Name:     CSRFCookie,
				Value:    token,
				Path:     path,
				HttpOnly: true,
				Secure:   c.Secure,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, token)))
	})
}

// CSRFToken returns the token of the current request, for embedding in forms
// and htmx headers.
func CSRFToken(ctx context.Context) string {
	token, _ := ctx.Value(csrfKey{}).(string)
	return token
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return hex.EncodeToString(b)
}

func validCSRFToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
