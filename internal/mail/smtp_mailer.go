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

	resetURLTemplate       string
	emailChangeURLTemplate string
}

func NewSMTPMailer(host, port, username, password, from, resetURLTemplate, emailChangeURLTemplate string) *SMTPMailer {
	return &SMTPMailer{
		host:                   host,
		port:                   port,
		username:               username,
		password:               password,
		from:                   from,
		resetURLTemplate:       resetURLTemplate,
		emailChangeURLTemplate: emailChangeURLTemplate,
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

	return m.send(toEmail, "Reset your Tamiyo password", body)
}

func (m *SMTPMailer) SendEmailChangeConfirmation(_ context.Context, toEmail, newEmail, token string) error {
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

	return m.send(toEmail, "Confirm your Tamiyo email change", body)
}

func (m *SMTPMailer) SendEmailChangedNotice(_ context.Context, toEmail string) error {
	body := "This address is now the email of your Tamiyo account: sign in with it from now on."

	return m.send(toEmail, "Your Tamiyo email was changed", body)
}

func (m *SMTPMailer) send(toEmail, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + toEmail,
		"Subject: " + subject,
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
