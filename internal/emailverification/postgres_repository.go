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
		INSERT INTO tamiyo.email_verification_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, added, expires_at
	`

	var created tokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.TokenHash, t.ExpiresAt); err != nil {
		return VerificationToken{}, err
	}

	return VerificationToken(created), nil
}

// Consume marks the token used and returns its user in one statement, so only one caller can win.
func (r *PostgresRepository) Consume(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.db.GetContext(ctx, &userID, `
		UPDATE tamiyo.email_verification_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id
	`, tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}
