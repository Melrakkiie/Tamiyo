package mail

import "context"

type Mailer interface {
	SendPasswordResetEmail(ctx context.Context, toEmail, resetToken string) error
}
