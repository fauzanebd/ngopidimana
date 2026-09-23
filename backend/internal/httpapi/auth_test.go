package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/auth"
	"github.com/fauzanebd/wheretowfc/backend/internal/ingestion"
	"github.com/fauzanebd/wheretowfc/backend/internal/recommendation"
)

type captureMailer struct{ messages []auth.Message }

func (mailer *captureMailer) Send(_ context.Context, message auth.Message) error {
	mailer.messages = append(mailer.messages, message)
	return nil
}

type testHealth struct{}

func (testHealth) Ping(context.Context) error { return nil }

func newTestServer(t *testing.T) (http.Handler, *captureMailer) {
	t.Helper()
	store := auth.NewMemoryStore()
	if _, err := store.AddContributor(context.Background(), "fauzanebd@gmail.com", "owner", ""); err != nil {
		t.Fatal(err)
	}
	mailer := &captureMailer{}
	interpreter, _ := recommendation.NewInterpreter("", "")
	return NewServer(Options{
		Recommendations: recommendation.NewService(interpreter),
		Ingestion:       ingestion.NewService(ingestion.NewMemoryStore(), testQueue{}),
		Health:          testHealth{},
		CORSOrigins:     "http://localhost:5174",
		Auth:            auth.NewService(store, mailer, "http://localhost:5174"),
		Cookies:         CookiePolicy{Name: "wfc_admin_session", TTL: time.Hour},
	}), mailer
}

func postJSON(handler http.Handler, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func getWithCookies(handler http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func signIn(t *testing.T, handler http.Handler, mailer *captureMailer) *http.Cookie {
	t.Helper()
	if response := postJSON(handler, "/v1/auth/request-link", `{"email":"fauzanebd@gmail.com"}`); response.Code != http.StatusAccepted {
		t.Fatalf("request-link = %d, want 202: %s", response.Code, response.Body.String())
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(mailer.messages))
	}
	const marker = "/auth/callback?token="
	index := strings.Index(mailer.messages[0].Text, marker)
	if index < 0 {
		t.Fatalf("no sign-in link in %q", mailer.messages[0].Text)
	}
	token := strings.TrimSpace(strings.SplitN(mailer.messages[0].Text[index+len(marker):], "\n", 2)[0])

	response := postJSON(handler, "/v1/auth/verify", `{"token":"`+token+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("verify = %d, want 200: %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "wfc_admin_session" || !cookies[0].HttpOnly {
		t.Fatalf("unexpected session cookie: %#v", cookies)
	}
	var payload struct {
		Contributor auth.Contributor `json:"contributor"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Contributor.Email != "fauzanebd@gmail.com" || payload.Contributor.Role != "owner" {
		t.Fatalf("unexpected contributor: %#v", payload.Contributor)
	}
	return cookies[0]
}

func TestAdminRoutesRejectAnonymousRequests(t *testing.T) {
	handler, _ := newTestServer(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/v1/admin/ingestion-runs"},
		{http.MethodPost, "/v1/admin/ingestion-runs"},
		{http.MethodPatch, "/v1/admin/ingestion-runs/ing_1"},
		{http.MethodDelete, "/v1/admin/ingestion-runs/ing_1"},
		{http.MethodGet, "/v1/admin/ingestion-runs/ing_1/google-place"},
		{http.MethodPost, "/v1/admin/ingestion-runs/ing_1/manual-evidence"},
		{http.MethodDelete, "/v1/admin/ingestion-runs/ing_1/manual-evidence/ev_1"},
		{http.MethodPatch, "/v1/admin/ingestion-runs/ing_1/evidence/ev_1"},
		{http.MethodDelete, "/v1/admin/ingestion-runs/ing_1/evidence/ev_1"},
		{http.MethodPost, "/v1/admin/ingestion-runs/ing_1/evidence/ev_1/restore"},
		{http.MethodPost, "/v1/admin/ingestion-runs/ing_1/evidence/ev_1/choose"},
		{http.MethodGet, "/v1/admin/anything-added-later"},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s = %d, want 401", route.method, route.path, response.Code)
		}
		if !strings.Contains(response.Body.String(), "sign in required") {
			t.Fatalf("%s %s body = %s", route.method, route.path, response.Body.String())
		}
	}
}

func TestSignInFlowUnlocksAdminRoutesAndLogoutRevokes(t *testing.T) {
	handler, mailer := newTestServer(t)
	if response := getWithCookies(handler, "/v1/admin/ingestion-runs"); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d, want 401", response.Code)
	}

	cookie := signIn(t, handler, mailer)

	if response := getWithCookies(handler, "/v1/auth/session", cookie); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "fauzanebd@gmail.com") {
		t.Fatalf("session = %d %s", response.Code, response.Body.String())
	}
	if response := getWithCookies(handler, "/v1/admin/ingestion-runs", cookie); response.Code != http.StatusOK {
		t.Fatalf("signed-in list = %d, want 200: %s", response.Code, response.Body.String())
	}

	logout := postJSON(handler, "/v1/auth/logout", `{}`, cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", logout.Code)
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("logout did not clear the cookie: %#v", cleared)
	}
	if response := getWithCookies(handler, "/v1/admin/ingestion-runs", cookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("list after logout = %d, want 401", response.Code)
	}
}

func TestVerifyRejectsUnknownAndReplayedTokens(t *testing.T) {
	handler, mailer := newTestServer(t)
	if response := postJSON(handler, "/v1/auth/verify", `{"token":"not-a-real-token"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("bogus token = %d, want 401", response.Code)
	}
	cookie := signIn(t, handler, mailer)
	if cookie.Value == "" {
		t.Fatal("no session token")
	}
	token := strings.TrimSpace(strings.SplitN(strings.SplitN(mailer.messages[0].Text, "/auth/callback?token=", 2)[1], "\n", 2)[0])
	if response := postJSON(handler, "/v1/auth/verify", `{"token":"`+token+`"}`); response.Code != http.StatusUnauthorized {
		t.Fatalf("replayed token = %d, want 401", response.Code)
	}
}

func TestRequestLinkNeverRevealsTheAllowList(t *testing.T) {
	handler, mailer := newTestServer(t)
	allowed := postJSON(handler, "/v1/auth/request-link", `{"email":"fauzanebd@gmail.com"}`)
	stranger := postJSON(handler, "/v1/auth/request-link", `{"email":"stranger@example.com"}`)
	if allowed.Code != http.StatusAccepted || stranger.Code != http.StatusAccepted {
		t.Fatalf("statuses = %d/%d, want 202/202", allowed.Code, stranger.Code)
	}
	if allowed.Body.String() != stranger.Body.String() {
		t.Fatalf("responses differ: %s vs %s", allowed.Body.String(), stranger.Body.String())
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("messages = %d, want 1 (only the invited address)", len(mailer.messages))
	}
}

func TestStateChangingEndpointsRequireJSONContentType(t *testing.T) {
	handler, mailer := newTestServer(t)
	cookie := signIn(t, handler, mailer)

	// A cross-site form can post these content types without a CORS preflight, so
	// they must never reach a handler — even with a valid session cookie.
	for _, testCase := range []struct{ contentType, body string }{
		{"text/plain", `{"action":"publish"}`},
		{"application/x-www-form-urlencoded", `action=publish`},
		{"", `{"action":"publish"}`},
	} {
		request := httptest.NewRequest(http.MethodPatch, "/v1/admin/ingestion-runs/ing_1", strings.NewReader(testCase.body))
		if testCase.contentType != "" {
			request.Header.Set("Content-Type", testCase.contentType)
		}
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("content type %q = %d, want 400", testCase.contentType, response.Code)
		}
	}
	if response := postJSON(handler, "/v1/auth/request-link", `{"email":"fauzanebd@gmail.com"}`); response.Code != http.StatusAccepted {
		t.Fatalf("application/json must still be accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestAdminPreflightNeedsNoSessionButAllowsCredentials(t *testing.T) {
	handler, _ := newTestServer(t)
	request := httptest.NewRequest(http.MethodOptions, "/v1/admin/ingestion-runs", nil)
	request.Header.Set("Origin", "http://localhost:5174")
	request.Header.Set("Access-Control-Request-Method", "GET")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5174" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}
}
