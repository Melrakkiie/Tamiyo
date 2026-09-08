package storage

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type storageRow struct {
	ID    int       `db:"id"`
	Name  string    `db:"name"`
	Type  string    `db:"type"`
	Added time.Time `db:"added"`
}

func (r storageRow) toDomain() Storage {
	return Storage{
		ID:    r.ID,
		Name:  r.Name,
		Type:  r.Type,
		Added: r.Added,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context) ([]Storage, error) {
	query := `
		SELECT id, name, type, added
		FROM tamiyo.storage
	`

	var rows []storageRow
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}

	storages := make([]Storage, 0, len(rows))
	for _, row := range rows {
		storages = append(storages, row.toDomain())
	}

	return storages, nil
}
