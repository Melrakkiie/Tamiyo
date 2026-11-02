package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// sendTimeout bounds one send, from dialing to the server's last answer.
const sendTimeout = 10 * time.Second

type SMTPMailer struct {
	logger  *zap.Logger
	timeout time.Duration

	host     string
	port     string
	username string
	password string
	from     string

	resetURLTemplate       string
	emailChangeURLTemplate string

	verificationURLTemplate string
}

func NewSMTPMailer(host, port, username, password, from, resetURLTemplate, emailChangeURLTemplate, verificationURLTemplate string, logger *zap.Logger) *SMTPMailer {
	return &SMTPMailer{
		logger:                  logger,
		timeout:                 sendTimeout,
		host:                    host,
		port:                    port,
		username:                username,
		password:                password,
		from:                    from,
		resetURLTemplate:        resetURLTemplate,
		emailChangeURLTemplate:  emailChangeURLTemplate,
		verificationURLTemplate: verificationURLTemplate,
	}
}

func (m *SMTPMailer) SendPasswordResetEmail(ctx context.Context, toEmail, resetToken string) error {
	var body string
	if m.resetURLTemplate != "" {
		link := fmt.Sprintf(m.resetURLTemplate, resetToken)
		body = fmt.Sprintf(
			"We received a request to reset your Tamiyo password.\r\n\r\nReset it here: %s\r\n\r\nIf you didn't request this, you can safely ignore this email.",
			link,
		)
	} else {
		body = fmt.Sprintf(
			"We received a request to reset your Tamiyo password.\r\n\r\nYour reset token:\r\n%s\r\n\r\nSubmit it to POST /auth/reset-password along with your new password. If you didn't request this, you can safely ignore this email.",
			resetToken,
		)
	}

	return m.send(ctx, "password_reset", toEmail, "Reset your Tamiyo password", body)
}

func (m *SMTPMailer) SendEmailChangeConfirmation(ctx context.Context, toEmail, newEmail, token string) error {
	var body string
	if m.emailChangeURLTemplate != "" {
		link := fmt.Sprintf(m.emailChangeURLTemplate, token)
		body = fmt.Sprintf(
			"We received a request to change the email of your Tamiyo account to %s.\r\n\r\nConfirm the change here: %s\r\n\r\nIf you didn't request this, ignore this email and change your password: nothing changes until the link is opened.",
			newEmail, link,
		)
	} else {
		body = fmt.Sprintf(
			"We received a request to change the email of your Tamiyo account to %s.\r\n\r\nYour confirmation token:\r\n%s\r\n\r\nSubmit it to POST /auth/confirm-email. If you didn't request this, ignore this email and change your password: nothing changes until the token is used.",
			newEmail, token,
		)
	}

	return m.send(ctx, "email_change_confirmation", toEmail, "Confirm your Tamiyo email change", body)
}

func (m *SMTPMailer) SendEmailVerification(ctx context.Context, toEmail, verificationToken string) error {
	var body string
	if m.verificationURLTemplate != "" {
		body = fmt.Sprintf(
			"Welcome to Tamiyo! Confirm your email address here: %s\r\n\r\nIf you didn't create a Tamiyo account, you can safely ignore this email.",
			fmt.Sprintf(m.verificationURLTemplate, verificationToken),
		)
	} else {
		body = fmt.Sprintf(
			"Welcome to Tamiyo! Your verification token:\r\n%s\r\n\r\nSubmit it to POST /auth/verify-email. If you didn't create a Tamiyo account, you can safely ignore this email.",
			verificationToken,
		)
	}

	return m.send(ctx, "email_verification", toEmail, "Confirm your Tamiyo email address", body)
}

// send is the single place where a failed email is logged: kind names the
// flow, and neither the token nor the recipient's address is written
// (a server's refusal can quote the address, so it is masked).
func (m *SMTPMailer) send(ctx context.Context, kind, toEmail, subject, body string) error {
	err := m.deliver(ctx, toEmail, subject, body)
	if err != nil {
		m.logger.Warn("could not send email",
			zap.String("kind", kind),
			zap.String("error", strings.ReplaceAll(err.Error(), toEmail, "<recipient>")),
		)
	}
	return err
}

// deliver does what smtp.SendMail does, but bounded: the dial and the whole
// exchange share one deadline, and cancelling ctx closes the connection.
func (m *SMTPMailer) deliver(ctx context.Context, toEmail, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + toEmail,
		"Subject: " + subject,
		"",
		body,
	}, "\r\n")

	dialer := net.Dialer{Timeout: m.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.host, m.port))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(m.timeout)); err != nil {
		return err
	}
	defer context.AfterFunc(ctx, func() { _ = conn.Close() })()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.host}); err != nil {
			return err
		}
	}
	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return err
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(toEmail); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}
