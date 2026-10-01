package mail

import (
	"context"

	"go.uber.org/zap"
)

// LoggingMailer logs the password-reset email instead of sending it. It's
// the default when SMTP_HOST isn't configured, so password recovery works
// out of the box in local/dev environments without a real mail server — the
// reset token shows up in the application logs instead of an inbox.
type LoggingMailer struct {
	logger *zap.Logger
}

func NewLoggingMailer(logger *zap.Logger) *LoggingMailer {
	return &LoggingMailer{logger: logger}
}

func (m *LoggingMailer) SendPasswordResetEmail(_ context.Context, toEmail, resetToken string) error {
	m.logger.Warn("SMTP_HOST not configured — logging password reset email instead of sending it",
		zap.String("to", toEmail),
		zap.String("reset_token", resetToken),
	)
	return nil
}
