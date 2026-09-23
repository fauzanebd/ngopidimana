// Package auth issues and verifies the email magic links that let invited
// contributors into the catalogue admin app.
//
// The flow is deliberately narrow: an address must already be on the allow-list,
// the link is single-use and short-lived, and a successful sign-in yields an
// opaque session token whose hash — never the token itself — is persisted.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"
)

const (
	// LoginTokenTTL is how long a sign-in link stays usable.
	LoginTokenTTL = 15 * time.Minute

	loginWindow          = 15 * time.Minute
	loginsPerWindow      = 3
	defaultSessionTTL    = 720 * time.Hour
	sessionTokenByteSize = 32
)

var (
	// ErrInvalidToken covers unknown, spent, and expired sign-in links alike:
	// the caller must not learn which of the three it was.
	ErrInvalidToken = errors.New("this sign-in link is invalid or has expired")
	// ErrNoSession means the request carried no usable session.
	ErrNoSession = errors.New("sign in required")
)

// Contributor is an address allowed into the admin app.
type Contributor struct {
	Email       string `json:"email"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name,omitempty"`
}

// Session is what a successful sign-in hands back.
type Session struct {
	Contributor Contributor
	Token       string
	ExpiresAt   time.Time
}

// Store persists contributors, sign-in tokens, and sessions. Implementations
// must treat every token as already hashed.
type Store interface {
	Contributor(ctx context.Context, email string) (Contributor, bool, error)
	CountRecentLoginTokens(ctx context.Context, email string, since time.Time) (int, error)
	SaveLoginToken(ctx context.Context, tokenHash []byte, email string, expiresAt time.Time) error
	ConsumeLoginToken(ctx context.Context, tokenHash []byte, now time.Time) (Contributor, error)
	CreateSession(ctx context.Context, tokenHash []byte, email string, expiresAt time.Time, userAgent string) error
	SessionContributor(ctx context.Context, tokenHash []byte, now time.Time) (Contributor, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

type Service struct {
	store      Store
	mailer     Mailer
	adminURL   string
	sessionTTL time.Duration
	now        func() time.Time
	random     io.Reader
}

func NewService(store Store, mailer Mailer, adminURL string) *Service {
	return &Service{
		store: store, mailer: mailer, adminURL: strings.TrimRight(strings.TrimSpace(adminURL), "/"),
		sessionTTL: defaultSessionTTL, now: time.Now, random: rand.Reader,
	}
}

// WithSessionTTL overrides how long a signed-in session lasts.
func (s *Service) WithSessionTTL(ttl time.Duration) *Service {
	if ttl > 0 {
		s.sessionTTL = ttl
	}
	return s
}

// RequestLink emails a sign-in link when the address is on the allow-list.
//
// Unknown addresses, malformed addresses, and addresses over the request limit
// all return nil: the endpoint's answer must be identical in every case so it
// cannot be used to enumerate contributors.
func (s *Service) RequestLink(ctx context.Context, rawEmail string) error {
	email, valid := NormalizeEmail(rawEmail)
	if !valid {
		return nil
	}
	contributor, found, err := s.store.Contributor(ctx, email)
	if err != nil || !found {
		return err
	}
	now := s.now()
	recent, err := s.store.CountRecentLoginTokens(ctx, email, now.Add(-loginWindow))
	if err != nil {
		return err
	}
	if recent >= loginsPerWindow {
		return nil
	}
	token, err := s.newToken()
	if err != nil {
		return err
	}
	if err := s.store.SaveLoginToken(ctx, hashToken(token), email, now.Add(LoginTokenTTL)); err != nil {
		return err
	}
	return s.mailer.Send(ctx, loginMessage(contributor, s.signInLink(token)))
}

// Verify spends a sign-in link and opens a session for the contributor.
func (s *Service) Verify(ctx context.Context, loginToken, userAgent string) (Session, error) {
	if strings.TrimSpace(loginToken) == "" {
		return Session{}, ErrInvalidToken
	}
	now := s.now()
	contributor, err := s.store.ConsumeLoginToken(ctx, hashToken(loginToken), now)
	if err != nil {
		return Session{}, err
	}
	token, err := s.newToken()
	if err != nil {
		return Session{}, err
	}
	expiresAt := now.Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, hashToken(token), contributor.Email, expiresAt, userAgent); err != nil {
		return Session{}, err
	}
	return Session{Contributor: contributor, Token: token, ExpiresAt: expiresAt}, nil
}

// Session resolves a session token to its contributor.
func (s *Service) Session(ctx context.Context, sessionToken string) (Contributor, error) {
	if strings.TrimSpace(sessionToken) == "" {
		return Contributor{}, ErrNoSession
	}
	return s.store.SessionContributor(ctx, hashToken(sessionToken), s.now())
}

// Logout drops a session. Unknown tokens are ignored so signing out twice is
// not an error.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if strings.TrimSpace(sessionToken) == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, hashToken(sessionToken))
}

func (s *Service) signInLink(token string) string {
	return fmt.Sprintf("%s/auth/callback?token=%s", s.adminURL, token)
}

func (s *Service) newToken() (string, error) {
	buffer := make([]byte, sessionTokenByteSize)
	if _, err := io.ReadFull(s.random, buffer); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// NormalizeEmail accepts a bare address and returns it lower-cased, reporting
// false for anything that could not be looked up — including display-name forms
// and values carrying header separators.
func NormalizeEmail(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
		return "", false
	}
	address, err := mail.ParseAddress(trimmed)
	if err != nil || address.Address != trimmed {
		return "", false
	}
	return strings.ToLower(address.Address), true
}
