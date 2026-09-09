package storage

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type storageRow struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Type      string    `db:"type"`
	CardCount int       `db:"card_count"`
	Added     time.Time `db:"added"`
	Updated   time.Time `db:"updated"`
}

func (r storageRow) toDomain() Storage {
	return Storage{
		ID:        r.ID,
		Name:      r.Name,
		Type:      r.Type,
		CardCount: r.CardCount,
		Added:     r.Added,
		Updated:   r.Updated,
	}
}

func toStorageRow(storage Storage) storageRow {
	return storageRow{
		Name: storage.Name,
		Type: storage.Type,
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
		SELECT
		    tamiyo.storage.id AS id,
		    tamiyo.storage.name AS name,
		    tamiyo.storage.type AS type,
		    tamiyo.storage.added AS added,
			tamiyo.storage.updated as updated,
		    COUNT(tamiyo.cards.id) AS card_count
		FROM tamiyo.storage
		LEFT JOIN tamiyo.cards ON tamiyo.storage.id = tamiyo.cards.storage_id
		GROUP BY tamiyo.storage.id, tamiyo.storage.name, tamiyo.storage.type, tamiyo.storage.added, tamiyo.storage.updated
		ORDER BY tamiyo.storage.updated DESC
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

func (r *PostgresRepository) Create(ctx context.Context, storage Storage) (Storage, error) {
	row := toStorageRow(storage)
	query := `
    	INSERT INTO tamiyo.storage (name, type)
     	VALUES (:name, :type)
      	RETURNING id, name, type, added, updated
	`
	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Storage{}, err
	}
	defer stmt.Close()

	var created storageRow
	if err := stmt.GetContext(ctx, &created, row); err != nil {
		return Storage{}, err
	}

	return created.toDomain(), nil
}
