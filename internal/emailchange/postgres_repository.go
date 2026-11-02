package emailchange

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type changeTokenRow struct {
	ID        string    `db:"id"`
	UserID    string    `db:"user_id"`
	NewEmail  string    `db:"new_email"`
	TokenHash string    `db:"token_hash"`
	Added     time.Time `db:"added"`
	ExpiresAt time.Time `db:"expires_at"`
}

func (r changeTokenRow) toDomain() ChangeToken {
	return ChangeToken(r)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t ChangeToken) (ChangeToken, error) {
	query := `
		INSERT INTO tamiyo.email_change_tokens (user_id, new_email, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, new_email, token_hash, added, expires_at
	`

	var created changeTokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.NewEmail, t.TokenHash, t.ExpiresAt); err != nil {
		return ChangeToken{}, err
	}

	return created.toDomain(), nil
}

// Consume marks the token used only if it is unused and unexpired, in one
// statement, so concurrent callers cannot both succeed.
func (r *PostgresRepository) Consume(ctx context.Context, tokenHash string) (userID, newEmail string, err error) {
	query := `
		UPDATE tamiyo.email_change_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id, new_email
	`

	var row struct {
		UserID   string `db:"user_id"`
		NewEmail string `db:"new_email"`
	}
	if err := r.db.GetContext(ctx, &row, query, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}

	return row.UserID, row.NewEmail, nil
}
