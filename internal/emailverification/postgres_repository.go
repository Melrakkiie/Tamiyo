package emailverification

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type tokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	Email     string    `db:"email"`
	TokenHash string    `db:"token_hash"`
	Added     time.Time `db:"added"`
	ExpiresAt time.Time `db:"expires_at"`
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t VerificationToken) (VerificationToken, error) {
	query := `
		WITH purge AS (
			DELETE FROM tamiyo.email_verification_tokens WHERE user_id = $1 AND expires_at < now()
		)
		INSERT INTO tamiyo.email_verification_tokens (user_id, email, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, email, token_hash, added, expires_at
	`

	var created tokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.Email, t.TokenHash, t.ExpiresAt); err != nil {
		return VerificationToken{}, err
	}

	return VerificationToken(created), nil
}

// Consume marks the token used and returns its user and address in one statement, so only one caller can win.
func (r *PostgresRepository) Consume(ctx context.Context, tokenHash string) (string, string, error) {
	var row struct {
		UserID string `db:"user_id"`
		Email  string `db:"email"`
	}
	err := r.db.GetContext(ctx, &row, `
		UPDATE tamiyo.email_verification_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id, email
	`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return row.UserID, row.Email, err
}
