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

func TestLoggingMailer_ImplementsMailer(t *testing.T) {
	var _ Mailer = (*LoggingMailer)(nil)
}
