package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingMailer struct{ messages []Message }

func (mailer *recordingMailer) Send(_ context.Context, message Message) error {
	mailer.messages = append(mailer.messages, message)
	return nil
}

func newTestService(t *testing.T) (*Service, *MemoryStore, *recordingMailer) {
	t.Helper()
	store := NewMemoryStore()
	if _, err := store.AddContributor(context.Background(), "fauzanebd@gmail.com", "owner", ""); err != nil {
		t.Fatal(err)
	}
	mailer := &recordingMailer{}
	return NewService(store, mailer, "http://localhost:5174"), store, mailer
}

// linkToken pulls the raw token back out of the emailed link, which is the only
// place it exists in plaintext.
func linkToken(t *testing.T, message Message) string {
	t.Helper()
	const marker = "/auth/callback?token="
	index := strings.Index(message.Text, marker)
	if index < 0 {
		t.Fatalf("sign-in link missing from message: %q", message.Text)
	}
	return strings.TrimSpace(strings.SplitN(message.Text[index+len(marker):], "\n", 2)[0])
}

func TestRequestLinkOnlyMailsInvitedContributors(t *testing.T) {
	service, _, mailer := newTestService(t)

	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(mailer.messages))
	}
	if mailer.messages[0].To != "fauzanebd@gmail.com" || !strings.Contains(mailer.messages[0].HTML, "/auth/callback?token=") {
		t.Fatalf("unexpected message: %#v", mailer.messages[0])
	}

	// An address that is not on the allow-list must look exactly like a hit:
	// no mail, no error, so the endpoint cannot enumerate contributors.
	if err := service.RequestLink(t.Context(), "stranger@example.com"); err != nil {
		t.Fatalf("unknown address returned %v, want nil", err)
	}
	if err := service.RequestLink(t.Context(), "not-an-address"); err != nil {
		t.Fatalf("malformed address returned %v, want nil", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("messages = %d, want 1: unknown addresses must not be mailed", len(mailer.messages))
	}
}

func TestRequestLinkIsRateLimitedPerAddress(t *testing.T) {
	service, _, mailer := newTestService(t)
	for range loginsPerWindow {
		if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
			t.Fatal(err)
		}
	}
	if len(mailer.messages) != loginsPerWindow {
		t.Fatalf("messages = %d, want %d", len(mailer.messages), loginsPerWindow)
	}
	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	if len(mailer.messages) != loginsPerWindow {
		t.Fatalf("rate limit did not hold: messages = %d", len(mailer.messages))
	}
}

func TestVerifySpendsTheLinkExactlyOnce(t *testing.T) {
	service, _, mailer := newTestService(t)
	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	token := linkToken(t, mailer.messages[0])

	session, err := service.Verify(t.Context(), token, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	if session.Contributor.Email != "fauzanebd@gmail.com" || session.Contributor.Role != "owner" || session.Token == "" {
		t.Fatalf("unexpected session: %#v", session)
	}
	if !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("session already expired: %v", session.ExpiresAt)
	}
	if _, err := service.Verify(t.Context(), token, "test-agent"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("replaying the link returned %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsExpiredLinks(t *testing.T) {
	service, _, mailer := newTestService(t)
	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	token := linkToken(t, mailer.messages[0])
	service.now = func() time.Time { return time.Now().Add(LoginTokenTTL + time.Minute) }
	if _, err := service.Verify(t.Context(), token, "test-agent"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired link returned %v, want ErrInvalidToken", err)
	}
}

func TestSessionResolvesAndLogoutRevokes(t *testing.T) {
	service, _, mailer := newTestService(t)
	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	session, err := service.Verify(t.Context(), linkToken(t, mailer.messages[0]), "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	contributor, err := service.Session(t.Context(), session.Token)
	if err != nil || contributor.Email != "fauzanebd@gmail.com" {
		t.Fatalf("session lookup = %#v, %v", contributor, err)
	}
	if _, err := service.Session(t.Context(), "not-a-session"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("unknown session returned %v, want ErrNoSession", err)
	}
	if err := service.Logout(t.Context(), session.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Session(t.Context(), session.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("session survived logout: %v", err)
	}
}

func TestExpiredSessionStopsWorking(t *testing.T) {
	service, _, mailer := newTestService(t)
	service.sessionTTL = time.Minute
	if err := service.RequestLink(t.Context(), "fauzanebd@gmail.com"); err != nil {
		t.Fatal(err)
	}
	session, err := service.Verify(t.Context(), linkToken(t, mailer.messages[0]), "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := service.Session(t.Context(), session.Token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session returned %v, want ErrNoSession", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	for _, testCase := range []struct {
		input string
		want  string
		valid bool
	}{
		{"fauzanebd@gmail.com", "fauzanebd@gmail.com", true},
		{"  FauzanEBD@Gmail.com  ", "fauzanebd@gmail.com", true},
		{"Name <fauzanebd@gmail.com>", "", false},
		{"fauzanebd@gmail.com\r\nBcc: someone@example.com", "", false},
		{"", "", false},
		{"nope", "", false},
	} {
		got, valid := NormalizeEmail(testCase.input)
		if valid != testCase.valid || got != testCase.want {
			t.Fatalf("NormalizeEmail(%q) = %q, %v; want %q, %v", testCase.input, got, valid, testCase.want, testCase.valid)
		}
	}
}
