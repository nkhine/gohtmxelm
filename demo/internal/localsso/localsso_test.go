package localsso

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRedirectFlowCreatesSession(t *testing.T) {
	svc := New()
	tokens := []string{"state-token", "auth-code", "session-token"}
	svc.token = func() (string, error) {
		if len(tokens) == 0 {
			t.Fatal("unexpected token request")
		}
		tok := tokens[0]
		tokens = tokens[1:]
		return tok, nil
	}
	svc.now = func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) }
	var loginClaims Claims
	svc.OnLogin = func(claims Claims) { loginClaims = claims }

	startReq := httptest.NewRequest(http.MethodGet, "/api/sso/start", nil)
	startRec := httptest.NewRecorder()
	svc.Start(startRec, startReq)

	if got := startRec.Code; got != http.StatusFound {
		t.Fatalf("start status = %d, want 302", got)
	}
	stateCookie := findCookie(startRec.Result().Cookies(), StateCookie)
	if stateCookie == nil || stateCookie.Value != "state-token" || !stateCookie.HttpOnly {
		t.Fatalf("state cookie = %+v", stateCookie)
	}
	loc := startRec.Result().Header.Get("Location")
	if !strings.HasPrefix(loc, idpLoginPath+"?") {
		t.Fatalf("start location = %q", loc)
	}

	form := url.Values{}
	form.Set("state", "state-token")
	form.Set("redirect_uri", callbackPath)
	form.Set("email", "org-admin@customer-a.local")
	loginReq := httptest.NewRequest(http.MethodPost, idpLoginPath, strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	svc.IdPLogin(loginRec, loginReq)

	if got := loginRec.Code; got != http.StatusFound {
		t.Fatalf("login status = %d, want 302", got)
	}
	callbackURL, err := url.Parse(loginRec.Result().Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if callbackURL.Path != callbackPath || callbackURL.Query().Get("code") != "auth-code" {
		t.Fatalf("callback location = %q", callbackURL.String())
	}

	callbackReq := httptest.NewRequest(http.MethodGet, callbackURL.String(), nil)
	callbackReq.AddCookie(stateCookie)
	callbackRec := httptest.NewRecorder()
	svc.Callback(callbackRec, callbackReq)

	if got := callbackRec.Code; got != http.StatusFound {
		t.Fatalf("callback status = %d, want 302", got)
	}
	sessionCookie := findCookie(callbackRec.Result().Cookies(), SessionCookie)
	if sessionCookie == nil || sessionCookie.Value != "session-token" || !sessionCookie.HttpOnly {
		t.Fatalf("session cookie = %+v", sessionCookie)
	}
	sessionReq := httptest.NewRequest(http.MethodGet, "/api/sso/session", nil)
	sessionReq.AddCookie(sessionCookie)
	claims, ok := svc.Session(sessionReq)
	if !ok {
		t.Fatal("expected session")
	}
	if claims.Email != "org-admin@customer-a.local" || claims.Org != "customer-a" {
		t.Fatalf("claims = %+v", claims)
	}
	if loginClaims.Email != "org-admin@customer-a.local" {
		t.Fatalf("login hook claims = %+v", loginClaims)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	svc := New()
	svc.states["state-token"] = stateRecord{RedirectURI: callbackPath, CreatedAt: time.Now()}
	svc.codes["auth-code"] = codeRecord{Claims: svc.users["org-admin@customer-a.local"], CreatedAt: time.Now()}

	req := httptest.NewRequest(http.MethodGet, callbackPath+"?state=state-token&code=auth-code", nil)
	req.AddCookie(&http.Cookie{Name: StateCookie, Value: "different-state"})
	rec := httptest.NewRecorder()

	svc.Callback(rec, req)

	if got := rec.Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got)
	}
	if _, ok := svc.sessions["auth-code"]; ok {
		t.Fatal("unexpected session")
	}
}

func TestLogoutClearsSession(t *testing.T) {
	svc := New()
	svc.sessions["session-token"] = svc.users["deal-reader@customer-a.local"]
	loggedOut := false
	svc.OnLogout = func() { loggedOut = true }
	req := httptest.NewRequest(http.MethodPost, "/api/sso/logout", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "session-token"})
	rec := httptest.NewRecorder()

	svc.Logout(rec, req)

	if _, ok := svc.sessions["session-token"]; ok {
		t.Fatal("session was not removed")
	}
	cookie := findCookie(rec.Result().Cookies(), SessionCookie)
	if cookie == nil || cookie.MaxAge != -1 {
		t.Fatalf("clear cookie = %+v", cookie)
	}
	if !loggedOut {
		t.Fatal("logout hook was not called")
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
