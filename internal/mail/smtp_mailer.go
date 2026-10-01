package mail

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

type SMTPMailer struct {
	host     string
	port     string
	username string
	password string
	from     string

	resetURLTemplate string
}

func NewSMTPMailer(host, port, username, password, from, resetURLTemplate string) *SMTPMailer {
	return &SMTPMailer{
		host:             host,
		port:             port,
		username:         username,
		password:         password,
		from:             from,
		resetURLTemplate: resetURLTemplate,
	}
}

func (m *SMTPMailer) SendPasswordResetEmail(_ context.Context, toEmail, resetToken string) error {
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

	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + toEmail,
		"Subject: Reset your Tamiyo password",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}

	addr := m.host + ":" + m.port
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg))
}
