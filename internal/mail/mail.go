package mail

import "context"

type Mailer interface {
	SendPasswordResetEmail(ctx context.Context, toEmail, resetToken string) error
	SendEmailChangeConfirmation(ctx context.Context, toEmail, newEmail, token string) error
	SendEmailChangedNotice(ctx context.Context, toEmail string) error
}
