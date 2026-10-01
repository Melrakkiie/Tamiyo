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
}

func (r userRow) toDomain() User {
	return User{
		ID:           r.ID,
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Added:        r.Added,
		Updated:      r.Updated,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, u User) (User, error) {
	query := `
		INSERT INTO tamiyo.users (email, password_hash)
		VALUES (:email, :password_hash)
		RETURNING id, email, password_hash, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return User{}, err
	}
	defer stmt.Close()

	var created userRow
	if err := stmt.GetContext(ctx, &created, u); err != nil {
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
		SELECT id, email, password_hash, added, updated
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
