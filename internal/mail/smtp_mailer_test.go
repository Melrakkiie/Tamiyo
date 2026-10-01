package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSMTPServer speaks just enough SMTP to let net/smtp.SendMail complete
// a single send: EHLO, optional AUTH PLAIN, MAIL FROM, RCPT TO, DATA, QUIT.
// It captures the message body and, when auth is required, the decoded
// AUTH PLAIN credentials, for the test to assert on.
type fakeSMTPServer struct {
	listener     net.Listener
	requireAuth  bool
	received     chan string
	authReceived chan string
}

func startFakeSMTPServer(t *testing.T, requireAuth bool) (*fakeSMTPServer, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &fakeSMTPServer{
		listener:    ln,
		requireAuth: requireAuth,
		received:    make(chan string, 1),
	}
	if requireAuth {
		srv.authReceived = make(chan string, 1)
	}

	go srv.serveOne()
	t.Cleanup(func() { _ = ln.Close() })

	return srv, ln.Addr().String()
}

func (s *fakeSMTPServer) serveOne() {
	conn, err := s.listener.Accept()
	if err != nil {
		return // listener closed by test cleanup
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writeLine := func(line string) {
		_, _ = conn.Write([]byte(line + "\r\n"))
	}

	writeLine("220 localhost ESMTP fake")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			if s.requireAuth {
				_, _ = conn.Write([]byte("250-localhost\r\n250 AUTH PLAIN\r\n"))
			} else {
				writeLine("250 localhost")
			}
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			parts := strings.SplitN(line, " ", 3)
			if len(parts) == 3 && s.authReceived != nil {
				decoded, _ := base64.StdEncoding.DecodeString(parts[2])
				s.authReceived <- string(decoded)
			}
			writeLine("235 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM"):
			writeLine("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			writeLine("250 OK")
		case upper == "DATA":
			writeLine("354 Start mail input")
			var msg strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if dataLine == ".\r\n" || dataLine == ".\n" {
					break
				}
				msg.WriteString(dataLine)
			}
			s.received <- msg.String()
			writeLine("250 Queued")
		case upper == "QUIT":
			writeLine("221 Bye")
			return
		default:
			writeLine("500 unrecognized command")
		}
	}
}

func splitHostPort(t *testing.T, addr string) (string, string) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	return host, port
}

func TestSMTPMailer_SendPasswordResetEmail_UsesResetURLTemplateWhenSet(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	_ = srv
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "https://app.example.com/reset?token=%s")

	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.NoError(t, err)

	msg := <-srv.received
	assert.Contains(t, msg, "From: tamiyo@example.com")
	assert.Contains(t, msg, "To: alice@example.com")
	assert.Contains(t, msg, "Subject: Reset your Tamiyo password")
	assert.Contains(t, msg, "https://app.example.com/reset?token=reset-token-123")
}

func TestSMTPMailer_SendPasswordResetEmail_UsesRawTokenWhenNoTemplate(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "")

	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.NoError(t, err)

	msg := <-srv.received
	assert.Contains(t, msg, "reset-token-123")
	assert.Contains(t, msg, "POST /auth/reset-password")
}

func TestSMTPMailer_SendPasswordResetEmail_AuthenticatesWhenUsernameSet(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, true)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "smtp-user", "smtp-pass", "tamiyo@example.com", "")

	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.NoError(t, err)

	creds := <-srv.authReceived
	// PLAIN auth payload is "\x00<username>\x00<password>"
	assert.Equal(t, "\x00smtp-user\x00smtp-pass", creds)
}

func TestSMTPMailer_SendPasswordResetEmail_ConnectionFailureReturnsError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // nothing is listening here anymore

	host, port := splitHostPort(t, addr)
	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "")

	err = mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.Error(t, err)
}

func TestNewSMTPMailer_ImplementsMailer(t *testing.T) {
	var _ Mailer = (*SMTPMailer)(nil)
}
