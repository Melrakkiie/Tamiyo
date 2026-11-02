package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "https://app.example.com/reset?token=%s", "", "", zap.NewNop())

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

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "", zap.NewNop())

	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.NoError(t, err)

	msg := <-srv.received
	assert.Contains(t, msg, "reset-token-123")
	assert.Contains(t, msg, "POST /auth/reset-password")
}

func TestSMTPMailer_SendPasswordResetEmail_AuthenticatesWhenUsernameSet(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, true)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "smtp-user", "smtp-pass", "tamiyo@example.com", "", "", "", zap.NewNop())

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
	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "", zap.NewNop())

	err = mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")
	require.Error(t, err)
}

func TestSMTPMailer_SendEmailChangeConfirmation_UsesEmailChangeURLTemplateWhenSet(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "https://app.example.com/reset?token=%s", "https://app.example.com/confirm-email?token=%s", "", zap.NewNop())

	err := mailer.SendEmailChangeConfirmation(context.Background(), "alice@example.com", "new@example.com", "change-token-123")
	require.NoError(t, err)

	msg := <-srv.received
	assert.Contains(t, msg, "To: alice@example.com")
	assert.Contains(t, msg, "Subject: Confirm your Tamiyo email change")
	assert.Contains(t, msg, "new@example.com")
	assert.Contains(t, msg, "https://app.example.com/confirm-email?token=change-token-123")
}

func TestSMTPMailer_SendEmailChangeConfirmation_UsesRawTokenWhenNoTemplate(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "", zap.NewNop())

	err := mailer.SendEmailChangeConfirmation(context.Background(), "alice@example.com", "new@example.com", "change-token-123")
	require.NoError(t, err)

	msg := <-srv.received
	assert.Contains(t, msg, "change-token-123")
	assert.Contains(t, msg, "POST /auth/confirm-email")
}

func TestSMTPMailer_SendEmailVerification_UsesVerificationURLTemplateWhenSet(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "https://app.example.com/verify-email?token=%s", zap.NewNop())

	require.NoError(t, mailer.SendEmailVerification(context.Background(), "alice@example.com", "verify-token-123"))

	msg := <-srv.received
	assert.Contains(t, msg, "To: alice@example.com")
	assert.Contains(t, msg, "Subject: Confirm your Tamiyo email address")
	assert.Contains(t, msg, "https://app.example.com/verify-email?token=verify-token-123")
	assert.Contains(t, msg, "ignore this email")
}

func TestSMTPMailer_SendEmailVerification_UsesRawTokenWhenNoTemplate(t *testing.T) {
	srv, addr := startFakeSMTPServer(t, false)
	host, port := splitHostPort(t, addr)

	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "", zap.NewNop())

	require.NoError(t, mailer.SendEmailVerification(context.Background(), "alice@example.com", "verify-token-123"))

	msg := <-srv.received
	assert.Contains(t, msg, "verify-token-123")
	assert.Contains(t, msg, "POST /auth/verify-email")
	assert.Contains(t, msg, "ignore this email")
}

// startSilentSMTPServer accepts connections and never answers: the
// failure mode of an SMTP server that has hung after the TCP handshake.
func startSilentSMTPServer(t *testing.T) (string, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, conn)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
	})

	return splitHostPort(t, ln.Addr().String())
}

func newObservedMailer(t *testing.T, timeout time.Duration) (*SMTPMailer, *observer.ObservedLogs) {
	t.Helper()
	host, port := startSilentSMTPServer(t)
	core, logs := observer.New(zap.WarnLevel)
	mailer := NewSMTPMailer(host, port, "", "", "tamiyo@example.com", "", "", "", zap.New(core))
	mailer.timeout = timeout
	return mailer, logs
}

func TestSMTPMailer_Send_GivesUpWhenTheServerStopsAnswering(t *testing.T) {
	mailer, _ := newObservedMailer(t, 200*time.Millisecond)

	start := time.Now()
	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")

	require.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestSMTPMailer_Send_StopsWhenTheContextIsCancelled(t *testing.T) {
	mailer, _ := newObservedMailer(t, time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := mailer.SendPasswordResetEmail(ctx, "alice@example.com", "reset-token-123")

	require.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestSMTPMailer_Send_LogsEachFailureWithItsKindAndNeitherTokenNorAddress(t *testing.T) {
	sends := map[string]func(m *SMTPMailer) error{
		"password_reset": func(m *SMTPMailer) error {
			return m.SendPasswordResetEmail(context.Background(), "alice@example.com", "secret-token")
		},
		"email_change_confirmation": func(m *SMTPMailer) error {
			return m.SendEmailChangeConfirmation(context.Background(), "alice@example.com", "new@example.com", "secret-token")
		},
		"email_verification": func(m *SMTPMailer) error {
			return m.SendEmailVerification(context.Background(), "alice@example.com", "secret-token")
		},
	}

	for kind, send := range sends {
		t.Run(kind, func(t *testing.T) {
			mailer, logs := newObservedMailer(t, 100*time.Millisecond)

			require.Error(t, send(mailer))

			entries := logs.All()
			require.Len(t, entries, 1)
			assert.Equal(t, kind, entries[0].ContextMap()["kind"])
			logged := entries[0].Message + " " + toString(entries[0].ContextMap())
			assert.NotContains(t, logged, "secret-token")
			assert.NotContains(t, logged, "alice@example.com")
			assert.NotContains(t, logged, "new@example.com")
		})
	}
}

func toString(m map[string]any) string {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k + "=" + fmt.Sprint(v) + " ")
	}
	return b.String()
}

func TestNewSMTPMailer_ImplementsMailer(t *testing.T) {
	var _ Mailer = (*SMTPMailer)(nil)
}
