package user

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type userRow struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	PasswordHash string    `db:"password_hash"`
	Added        time.Time `db:"added"`
	Updated      time.Time `db:"updated"`
	DisplayName  *string   `db:"display_name"`
}

func (r userRow) toDomain() User {
	return User(r)
}

func toUserRow(u User) userRow {
	return userRow{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, u User) (User, error) {
	row := toUserRow(u)
	query := `
		INSERT INTO tamiyo.users (email, password_hash, display_name)
		VALUES (:email, :password_hash, :display_name)
		RETURNING id, email, password_hash, added, updated, display_name
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return User{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

	var created userRow
	if err := stmt.GetContext(ctx, &created, row); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return User{}, ErrEmailAlreadyTaken
		}
		return User{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	query := `
		SELECT id, email, password_hash, added, updated, display_name
		FROM tamiyo.users
		WHERE email = $1
	`

	var row userRow
	if err := r.db.GetContext(ctx, &row, query, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (User, error) {
	query := `
		SELECT id, email, password_hash, added, updated, display_name
		FROM tamiyo.users
		WHERE id = $1
	`

	var row userRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	query := `UPDATE tamiyo.users SET password_hash = $1 WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, passwordHash, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *PostgresRepository) UpdateEmail(ctx context.Context, id string, email string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE tamiyo.users SET email = $1 WHERE id = $2`, email, id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrEmailAlreadyTaken
		}
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *PostgresRepository) UpdateDisplayName(ctx context.Context, id string, displayName *string) (User, error) {
	query := `
		UPDATE tamiyo.users SET display_name = $1 WHERE id = $2
		RETURNING id, email, password_hash, added, updated, display_name
	`

	var row userRow
	if err := r.db.GetContext(ctx, &row, query, displayName, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}

	return row.toDomain(), nil
}
