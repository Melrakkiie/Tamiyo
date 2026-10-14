package exchangerate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Find(ctx context.Context, currency string) (Rate, bool, error) {
	rate := Rate{Currency: currency}
	err := r.db.QueryRowContext(ctx, `
		SELECT per_eur::float8, rate_date FROM tamiyo.exchange_rates WHERE currency = $1
	`, currency).Scan(&rate.PerEUR, &rate.Date)
	if errors.Is(err, sql.ErrNoRows) {
		return Rate{}, false, nil
	}
	if err != nil {
		return Rate{}, false, err
	}
	return rate, true, nil
}

func (r *PostgresRepository) Save(ctx context.Context, rate Rate) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.exchange_rates (currency, per_eur, rate_date, refreshed_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (currency) DO UPDATE
		SET per_eur = EXCLUDED.per_eur, rate_date = EXCLUDED.rate_date, refreshed_at = now()
	`, rate.Currency, rate.PerEUR, rate.Date)
	return err
}
