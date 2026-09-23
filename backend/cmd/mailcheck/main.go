// Command mailcheck sends one message through the configured SMTP relay so an
// operator can prove SMTP_HOST / SMTP_USERNAME / SMTP_PASSWORD / SMTP_FROM and
// the sender validation are right before relying on sign-in mail.
//
//	mailcheck you@example.com
//
// It prints the relay's own error text on failure, which is where "sender not
// validated" and authentication rejections surface.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fauzanebd/wheretowfc/backend/internal/auth"
	"github.com/fauzanebd/wheretowfc/backend/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: mailcheck <recipient@example.com>")
	}
	recipient, valid := auth.NormalizeEmail(os.Args[1])
	if !valid {
		log.Fatalf("%q is not a usable email address", os.Args[1])
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		log.Fatal("SMTP_HOST is not set; fill in the SMTP settings in .env first (with SMTP_HOST empty the API logs sign-in links instead of sending them)")
	}
	mode := auth.SMTPTLSMode(cfg.SMTPPort, cfg.SMTPTLS)
	mailer := auth.MailerFor(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPFrom, cfg.SMTPTLS)

	message := auth.Message{
		To:      recipient,
		Subject: "Where to WFC mail check",
		Text:    fmt.Sprintf("Your SMTP relay is configured correctly.\n\nRelay: %s:%d (%s)\nFrom: %s\nSent: %s\n", cfg.SMTPHost, cfg.SMTPPort, mode, cfg.SMTPFrom, time.Now().Format(time.RFC3339)),
		HTML:    fmt.Sprintf(`<p>Your SMTP relay is configured correctly.</p><p style="color:#677166;font-size:13px">Relay: %s:%d (%s)<br>From: %s<br>Sent: %s</p>`, cfg.SMTPHost, cfg.SMTPPort, mode, cfg.SMTPFrom, time.Now().Format(time.RFC3339)),
	}
	if err := mailer.Send(context.Background(), message); err != nil {
		log.Fatalf("send via %s:%d (%s) failed: %v", cfg.SMTPHost, cfg.SMTPPort, mode, err)
	}
	fmt.Printf("sent to %s via %s:%d (%s) as %s\n", recipient, cfg.SMTPHost, cfg.SMTPPort, mode, cfg.SMTPFrom)
}
