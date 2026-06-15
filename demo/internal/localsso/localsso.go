package localsso

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookie = "gohtmxelm_sso_session"
	StateCookie   = "gohtmxelm_sso_state"

	startPath    = "/api/sso/start"
	idpLoginPath = "/api/sso/idp/login"
	callbackPath = "/api/sso/callback"
)

var (
	ErrMissingState = errors.New("missing state")
	ErrBadState     = errors.New("invalid state")
	ErrBadCode      = errors.New("invalid code")
)

// Claims are the identity attributes returned by the local SSO simulator.
type Claims struct {
	Subject  string
	Email    string
	Name     string
	Org      string
	Groups   []string
	AuthTime time.Time
}

type stateRecord struct {
	RedirectURI string
	CreatedAt   time.Time
}

type codeRecord struct {
	Claims    Claims
	CreatedAt time.Time
}

// Service owns the local-only SSO state used by the demo.
type Service struct {
	mu       sync.Mutex
	states   map[string]stateRecord
	codes    map[string]codeRecord
	sessions map[string]Claims
	users    map[string]Claims
	now      func() time.Time
	token    func() (string, error)

	OnLogin  func(Claims)
	OnLogout func()
}

// New returns a local SSO simulator with fixture users.
func New() *Service {
	return &Service{
		states:   map[string]stateRecord{},
		codes:    map[string]codeRecord{},
		sessions: map[string]Claims{},
		users: map[string]Claims{
			"org-admin@customer-a.local": {
				Subject: "user-org-admin",
				Email:   "org-admin@customer-a.local",
				Name:    "Org Admin",
				Org:     "customer-a",
				Groups:  []string{"OrgAdmin", "ProjectAdmin"},
			},
			"deal-reader@customer-a.local": {
				Subject: "user-deal-reader",
				Email:   "deal-reader@customer-a.local",
				Name:    "Deal Reader",
				Org:     "customer-a",
				Groups:  []string{"DealReader"},
			},
			"platform-admin@alcove.local": {
				Subject: "user-platform-admin",
				Email:   "platform-admin@alcove.local",
				Name:    "Platform Admin",
				Org:     "platform",
				Groups:  []string{"PlatformAdmin"},
			},
		},
		now:   time.Now,
		token: randomToken,
	}
}

// Start begins the browser redirect flow.
func (s *Service) Start(w http.ResponseWriter, r *http.Request) {
	state, err := s.token()
	if err != nil {
		http.Error(w, "could not create state", http.StatusInternalServerError)
		return
	}
	redirectURI := callbackPath
	s.mu.Lock()
	s.states[state] = stateRecord{RedirectURI: redirectURI, CreatedAt: s.now()}
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     StateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	q := url.Values{}
	q.Set("state", state)
	q.Set("redirect_uri", redirectURI)
	http.Redirect(w, r, idpLoginPath+"?"+q.Encode(), http.StatusFound)
}

// IdPLogin renders the local IdP form for GET and completes it for POST.
func (s *Service) IdPLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, r)
	case http.MethodPost:
		s.completeLogin(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// Callback verifies state/code, creates a local session, and redirects back to
// the demo page.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	claims, err := s.consumeCallback(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session, err := s.token()
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}

	claims.AuthTime = s.now().UTC()
	s.mu.Lock()
	s.sessions[session] = claims
	s.mu.Unlock()
	if s.OnLogin != nil {
		s.OnLogin(claims)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    session,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   3600,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/examples/sso-local?login=ok", http.StatusFound)
}

// Session reads the current local SSO session from the request.
func (s *Service) Session(r *http.Request) (Claims, bool) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		return Claims{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	claims, ok := s.sessions[cookie.Value]
	return claims, ok
}

// Logout clears the browser cookie and removes the in-memory session.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	if s.OnLogout != nil {
		s.OnLogout()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Service) renderLogin(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	redirectURI := strings.TrimSpace(r.URL.Query().Get("redirect_uri"))
	if state == "" || redirectURI == "" {
		http.Error(w, "missing state or redirect_uri", http.StatusBadRequest)
		return
	}
	if !s.validState(state, redirectURI) {
		http.Error(w, "unknown state", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Local SSO</title><style>`)
	b.WriteString(`body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f8fafc;color:#0f172a;font-family:system-ui,-apple-system,sans-serif}.panel{width:min(28rem,calc(100vw - 2rem));border:1px solid #d7dee8;border-radius:8px;background:white;box-shadow:0 1px 3px rgba(0,0,0,.12);padding:1rem}.eyebrow{font-size:.72rem;font-weight:850;text-transform:uppercase;letter-spacing:.06em;color:#64748b}h1{font-size:1.1rem;margin:.2rem 0 .8rem}label{display:block;font-size:.74rem;font-weight:850;color:#475569;margin:.7rem 0 .25rem}select,input{width:100%;box-sizing:border-box;border:1px solid #cbd5e1;border-radius:7px;padding:.5rem .6rem;font-size:.9rem}button{margin-top:.9rem;width:100%;border:0;border-radius:7px;background:#00ACD7;color:white;font-weight:850;padding:.55rem .7rem;cursor:pointer}.hint{font-size:.78rem;color:#64748b;line-height:1.4}.trace{margin-top:.8rem;padding:.55rem;border-radius:7px;background:#0f172a;color:#bae6fd;font-family:Menlo,Consolas,monospace;font-size:.72rem;overflow-wrap:anywhere}`)
	b.WriteString(`</style></head><body><main class="panel"><span class="eyebrow">Local identity provider simulator</span><h1>Approve SSO sign-in</h1><p class="hint">This page stands in for an external IdP such as Entra, Okta, or Dex. The app is no longer in control of the DOM at this point.</p>`)
	b.WriteString(`<form method="post" action="` + idpLoginPath + `">`)
	b.WriteString(`<input type="hidden" name="state" value="` + html.EscapeString(state) + `"><input type="hidden" name="redirect_uri" value="` + html.EscapeString(redirectURI) + `">`)
	b.WriteString(`<label for="email">User</label><select id="email" name="email">`)
	for _, email := range []string{"org-admin@customer-a.local", "deal-reader@customer-a.local", "platform-admin@alcove.local"} {
		b.WriteString(`<option value="` + html.EscapeString(email) + `">` + html.EscapeString(email) + `</option>`)
	}
	b.WriteString(`</select><button type="submit">Approve sign-in</button></form>`)
	b.WriteString(`<div class="trace">GET /api/sso/idp/login<br>state=` + html.EscapeString(state) + `<br>redirect_uri=` + html.EscapeString(redirectURI) + `</div>`)
	b.WriteString(`</main></body></html>`)
	_, _ = w.Write([]byte(b.String()))
}

func (s *Service) completeLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	state := strings.TrimSpace(r.FormValue("state"))
	redirectURI := strings.TrimSpace(r.FormValue("redirect_uri"))
	if !s.validState(state, redirectURI) {
		http.Error(w, "unknown state", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	user, ok := s.users[email]
	if !ok {
		http.Error(w, "unknown user", http.StatusUnauthorized)
		return
	}
	code, err := s.token()
	if err != nil {
		http.Error(w, "could not create code", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.codes[code] = codeRecord{Claims: user, CreatedAt: s.now()}
	s.mu.Unlock()

	q := url.Values{}
	q.Set("code", code)
	q.Set("state", state)
	http.Redirect(w, r, redirectURI+"?"+q.Encode(), http.StatusFound)
}

func (s *Service) consumeCallback(r *http.Request) (Claims, error) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		return Claims{}, ErrMissingState
	}
	cookie, err := r.Cookie(StateCookie)
	if err != nil || cookie.Value != state {
		return Claims{}, ErrBadState
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.states[state]; !ok {
		return Claims{}, ErrBadState
	}
	delete(s.states, state)
	rec, ok := s.codes[code]
	if !ok {
		return Claims{}, ErrBadCode
	}
	delete(s.codes, code)
	return rec.Claims, nil
}

func (s *Service) validState(state, redirectURI string) bool {
	if state == "" || redirectURI == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.states[state]
	return ok && rec.RedirectURI == redirectURI
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
