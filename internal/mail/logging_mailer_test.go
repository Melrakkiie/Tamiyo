package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggingMailer_SendPasswordResetEmail_LogsTokenAndReturnsNil(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	mailer := NewLoggingMailer(logger)

	err := mailer.SendPasswordResetEmail(context.Background(), "alice@example.com", "reset-token-123")

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())

	entry := logs.All()[0]
	assert.Equal(t, zap.WarnLevel, entry.Level)
	fields := entry.ContextMap()
	assert.Equal(t, "alice@example.com", fields["to"])
	assert.Equal(t, "reset-token-123", fields["reset_token"])
}

func TestLoggingMailer_SendEmailChangeConfirmation_LogsTokenAndReturnsNil(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	mailer := NewLoggingMailer(zap.New(core))

	err := mailer.SendEmailChangeConfirmation(context.Background(), "alice@example.com", "new@example.com", "change-token-123")

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	fields := logs.All()[0].ContextMap()
	assert.Equal(t, "alice@example.com", fields["to"])
	assert.Equal(t, "new@example.com", fields["new_email"])
	assert.Equal(t, "change-token-123", fields["email_change_token"])
}

func TestLoggingMailer_SendEmailChangedNotice_LogsAndReturnsNil(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	mailer := NewLoggingMailer(zap.New(core))

	err := mailer.SendEmailChangedNotice(context.Background(), "new@example.com")

	require.NoError(t, err)
	require.Equal(t, 1, logs.Len())
	fields := logs.All()[0].ContextMap()
	assert.Equal(t, "new@example.com", fields["to"])
}

func TestLoggingMailer_ImplementsMailer(t *testing.T) {
	var _ Mailer = (*LoggingMailer)(nil)
}
