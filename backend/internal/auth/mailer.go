package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const (
	dialTimeout     = 10 * time.Second
	messageBoundary = "wfc-mail-boundary-6d1f4a"
)

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Mailer interface {
	Send(ctx context.Context, message Message) error
}

// LogMailer writes messages to the process log instead of sending them. It is
// what you get when SMTP_HOST is empty, so a developer can complete a sign-in
// without a mail provider. It is announced loudly at startup and must never be
// used in production.
type LogMailer struct{}

func (LogMailer) Send(_ context.Context, message Message) error {
	log.Printf("no SMTP configured, mail to %s not sent — %s\n%s", message.To, message.Subject, message.Text)
	return nil
}

// SMTPTLSMode resolves the configured transport, mapping the conventional port
// 465 onto implicit TLS. Anything unrecognised resolves to STARTTLS: a typo in
// SMTP_TLS must never silently downgrade the connection to plaintext.
func SMTPTLSMode(port int, tlsMode string) string {
	switch strings.ToLower(strings.TrimSpace(tlsMode)) {
	case "none":
		return "none"
	case "implicit":
		return "implicit"
	default:
		if port == 465 {
			return "implicit"
		}
		return "starttls"
	}
}

// MailerFor builds the mailer the API and the mail check both use: a log-only
// mailer when no SMTP host is configured, otherwise an SMTP mailer.
func MailerFor(host string, port int, username, password, from, tlsMode string) Mailer {
	if strings.TrimSpace(host) == "" {
		return LogMailer{}
	}
	return NewSMTPMailer(host, port, username, password, from, SMTPTLSMode(port, tlsMode))
}

// SMTPMailer delivers over SMTP. tlsMode is one of:
//
//	"starttls" — connect in the clear, then upgrade (submission port 587)
//	"implicit" — TLS from the first byte (port 465)
//	"none"     — no TLS at all; only for a local relay such as Mailpit
type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
	tlsMode  string
}

func NewSMTPMailer(host string, port int, username, password, from, tlsMode string) SMTPMailer {
	return SMTPMailer{host: host, port: port, username: username, password: password, from: from, tlsMode: tlsMode}
}

func (mailer SMTPMailer) Send(ctx context.Context, message Message) error {
	address := net.JoinHostPort(mailer.host, strconv.Itoa(mailer.port))
	dialer := &net.Dialer{Timeout: dialTimeout}
	tlsConfig := &tls.Config{ServerName: mailer.host, MinVersion: tls.VersionTLS12}
	var connection net.Conn
	var err error
	if mailer.tlsMode == "implicit" {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", address, err)
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, mailer.host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()
	if mailer.tlsMode != "none" {
		// Anything that is not an explicit opt-out must encrypt: an unrecognised
		// mode reaching here fails loudly instead of sending in the clear.
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return errors.New("smtp server does not offer STARTTLS; use SMTP_TLS=implicit or SMTP_TLS=none if that is intended")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if mailer.username != "" {
		if err := client.Auth(smtp.PlainAuth("", mailer.username, mailer.password, mailer.host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(envelopeAddress(mailer.from)); err != nil {
		return fmt.Errorf("smtp envelope from: %w", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return fmt.Errorf("smtp envelope to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(renderMessage(mailer.from, message)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

// envelopeAddress reduces `Name <address>` to the bare address SMTP commands
// require, falling back to the input when it is already bare.
func envelopeAddress(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return value
}

func renderMessage(from string, message Message) []byte {
	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
		from, message.To, message.Subject, time.Now().Format(time.RFC1123Z), messageBoundary)
	fmt.Fprintf(&buffer, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", messageBoundary, message.Text)
	fmt.Fprintf(&buffer, "--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n", messageBoundary, message.HTML)
	fmt.Fprintf(&buffer, "--%s--\r\n", messageBoundary)
	return buffer.Bytes()
}

func loginMessage(contributor Contributor, link string) Message {
	return Message{
		To:      contributor.Email,
		Subject: "Your Where to WFC admin sign-in link",
		Text: fmt.Sprintf("Open the catalogue admin:\n\n%s\n\nThe link works once and expires in %d minutes. If you did not request it, ignore this email.\n",
			link, int(LoginTokenTTL.Minutes())),
		HTML: fmt.Sprintf(`<div style="font-family:ui-sans-serif,system-ui,-apple-system,'Segoe UI',sans-serif;color:#1d2a20;line-height:1.6">
  <p>Here is your sign-in link for the Where to WFC catalogue admin.</p>
  <p style="margin:24px 0"><a href="%s" style="background:#294d37;color:#fffdf7;padding:12px 20px;border-radius:10px;text-decoration:none;font-weight:600;display:inline-block">Open the catalogue admin</a></p>
  <p style="color:#677166;font-size:13px">The link works once and expires in %d minutes. If you did not request it, ignore this email.</p>
</div>`, link, int(LoginTokenTTL.Minutes())),
	}
}
