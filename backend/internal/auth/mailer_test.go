package auth

import (
	"bufio"
	"context"
	"net"
	"net/textproto"
	"strings"
	"testing"
)

type receivedMail struct{ from, to, body string }

// fakeSMTPServer speaks just enough SMTP to capture one message. It advertises
// no STARTTLS, which is what a local relay such as Mailpit looks like.
func fakeSMTPServer(t *testing.T) (string, chan receivedMail) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	captured := make(chan receivedMail, 1)
	go func() {
		defer listener.Close()
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := textproto.NewReader(bufio.NewReader(connection))
		writer := bufio.NewWriter(connection)
		reply := func(line string) {
			_, _ = writer.WriteString(line + "\r\n")
			_ = writer.Flush()
		}
		reply("220 fake ESMTP")
		mail := receivedMail{}
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				reply("250-fake")
				reply("250 8BITMIME")
			case strings.HasPrefix(line, "MAIL FROM:"):
				mail.from = addressBetween(line)
				reply("250 ok")
			case strings.HasPrefix(line, "RCPT TO:"):
				mail.to = addressBetween(line)
				reply("250 ok")
			case line == "DATA":
				reply("354 go ahead")
				for {
					data, err := reader.ReadLine()
					if err != nil {
						return
					}
					if data == "." {
						break
					}
					mail.body += data + "\n"
				}
				reply("250 queued")
			case line == "QUIT":
				reply("221 bye")
				captured <- mail
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return listener.Addr().String(), captured
}

// addressBetween pulls the address out of `MAIL FROM:<a@b> BODY=8BITMIME`, whose
// trailing parameters depend on what the server advertises.
func addressBetween(line string) string {
	start := strings.Index(line, "<")
	end := strings.Index(line, ">")
	if start < 0 || end < start {
		return ""
	}
	return line[start+1 : end]
}

func splitAddress(t *testing.T, address string) (host, port string) {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestSMTPTLSModeResolution(t *testing.T) {
	for _, testCase := range []struct {
		port int
		mode string
		want string
	}{
		{587, "", "starttls"},
		{587, "starttls", "starttls"},
		{587, "STARTTLS", "starttls"},
		{465, "", "implicit"},
		{465, "starttls", "implicit"},
		{587, "implicit", "implicit"},
		{1025, "none", "none"},
		{587, "something-else", "starttls"},
	} {
		if got := SMTPTLSMode(testCase.port, testCase.mode); got != testCase.want {
			t.Fatalf("SMTPTLSMode(%d, %q) = %q, want %q", testCase.port, testCase.mode, got, testCase.want)
		}
	}
	if _, isLogMailer := MailerFor("", 587, "", "", "", "").(LogMailer); !isLogMailer {
		t.Fatal("MailerFor with no host should fall back to the log mailer")
	}
	if mailer, ok := MailerFor("smtp.example.com", 465, "key", "secret", "a@b.test", "").(SMTPMailer); !ok || mailer.tlsMode != "implicit" {
		t.Fatalf("MailerFor on port 465 = %#v, want implicit TLS", mailer)
	}
}

func TestSMTPMailerSendsTheSignInMessage(t *testing.T) {
	address, captured := fakeSMTPServer(t)
	host, port := splitAddress(t, address)
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatal(err)
	}
	mailer := NewSMTPMailer(host, portNumber, "", "", "Where to WFC <admin@wheretowfc.test>", "none")
	message := loginMessage(Contributor{Email: "fauzanebd@gmail.com"}, "https://admin.example.com/auth/callback?token=abc123")

	if err := mailer.Send(context.Background(), message); err != nil {
		t.Fatalf("send: %v", err)
	}
	mail := <-captured
	if mail.from != "admin@wheretowfc.test" {
		t.Fatalf("envelope from = %q, want the bare address", mail.from)
	}
	if mail.to != "fauzanebd@gmail.com" {
		t.Fatalf("envelope to = %q", mail.to)
	}
	for _, expected := range []string{
		"To: fauzanebd@gmail.com",
		"Subject: Your Where to WFC admin sign-in link",
		"Content-Type: multipart/alternative",
		"https://admin.example.com/auth/callback?token=abc123",
	} {
		if !strings.Contains(mail.body, expected) {
			t.Fatalf("message body is missing %q:\n%s", expected, mail.body)
		}
	}
}

func TestSMTPMailerRefusesToSendInTheClearWhenTLSIsExpected(t *testing.T) {
	address, _ := fakeSMTPServer(t)
	host, port := splitAddress(t, address)
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatal(err)
	}
	mailer := NewSMTPMailer(host, portNumber, "", "", "admin@wheretowfc.test", "starttls")
	err = mailer.Send(context.Background(), loginMessage(Contributor{Email: "fauzanebd@gmail.com"}, "https://admin.example.com/x"))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("send over a server without STARTTLS returned %v, want a STARTTLS error", err)
	}
}
