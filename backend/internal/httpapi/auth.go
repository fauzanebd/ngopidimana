package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/auth"
)

// AuthService is the subset of the auth service the HTTP layer needs.
type AuthService interface {
	RequestLink(ctx context.Context, email string) error
	Verify(ctx context.Context, loginToken, userAgent string) (auth.Session, error)
	Session(ctx context.Context, sessionToken string) (auth.Contributor, error)
	Logout(ctx context.Context, sessionToken string) error
}

// CookiePolicy describes the admin session cookie.
type CookiePolicy struct {
	Name     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
	TTL      time.Duration
}

// ParseSameSite maps a configured value onto the cookie attribute, defaulting to
// Lax so a missing setting stays safe.
func ParseSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

func (s *Server) requestLink(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if s.auth == nil {
		writeError(writer, http.StatusServiceUnavailable, "admin sign-in is not configured")
		return
	}
	if err := s.auth.RequestLink(request.Context(), input.Email); err != nil {
		// The client gets a neutral answer either way, but the operator needs the
		// relay's own words ("sender not validated", auth rejected) in the log.
		log.Printf("sign-in email failed: %v", err)
		writeError(writer, http.StatusBadGateway, "could not send the sign-in email")
		return
	}
	// The answer is identical for allowed and unknown addresses.
	writeJSON(writer, http.StatusAccepted, map[string]string{"status": "sent"})
}

func (s *Server) verifyLogin(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if s.auth == nil {
		writeError(writer, http.StatusServiceUnavailable, "admin sign-in is not configured")
		return
	}
	session, err := s.auth.Verify(request.Context(), input.Token, request.UserAgent())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			writeError(writer, http.StatusUnauthorized, err.Error())
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "could not complete sign-in")
		return
	}
	s.setSessionCookie(writer, session)
	writeJSON(writer, http.StatusOK, map[string]any{"contributor": session.Contributor})
}

func (s *Server) session(writer http.ResponseWriter, request *http.Request) {
	contributor, err := s.contributorFromRequest(request)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, auth.ErrNoSession.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"contributor": contributor})
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if s.auth != nil {
		if cookie, err := request.Cookie(s.cookieName()); err == nil {
			if err := s.auth.Logout(request.Context(), cookie.Value); err != nil {
				writeError(writer, http.StatusServiceUnavailable, "could not sign out")
				return
			}
		}
	}
	s.clearSessionCookie(writer)
	writer.WriteHeader(http.StatusNoContent)
}

// requireContributor guards the whole /v1/admin subtree. Registering it on the
// prefix rather than per route means a new admin route is protected by default.
func (s *Server) requireContributor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, err := s.contributorFromRequest(request); err != nil {
			writeError(writer, http.StatusUnauthorized, auth.ErrNoSession.Error())
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) contributorFromRequest(request *http.Request) (auth.Contributor, error) {
	if s.auth == nil {
		return auth.Contributor{}, auth.ErrNoSession
	}
	cookie, err := request.Cookie(s.cookieName())
	if err != nil || cookie.Value == "" {
		return auth.Contributor{}, auth.ErrNoSession
	}
	return s.auth.Session(request.Context(), cookie.Value)
}

func (s *Server) setSessionCookie(writer http.ResponseWriter, session auth.Session) {
	ttl := s.cookies.TTL
	if ttl <= 0 {
		ttl = time.Until(session.ExpiresAt)
	}
	http.SetCookie(writer, &http.Cookie{
		Name: s.cookieName(), Value: session.Token, Path: "/", Domain: s.cookies.Domain,
		MaxAge: int(ttl.Seconds()), HttpOnly: true, Secure: s.cookies.Secure, SameSite: s.cookies.SameSite,
	})
}

func (s *Server) clearSessionCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{
		Name: s.cookieName(), Value: "", Path: "/", Domain: s.cookies.Domain,
		MaxAge: -1, HttpOnly: true, Secure: s.cookies.Secure, SameSite: s.cookies.SameSite,
	})
}

func (s *Server) cookieName() string {
	if s.cookies.Name != "" {
		return s.cookies.Name
	}
	return "wfc_admin_session"
}
