package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type storageRow struct {
	ID        int       `db:"id"`
	UserID    string    `db:"user_id"`
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

func toStorageRow(userID string, storage Storage) storageRow {
	return storageRow{
		ID:     storage.ID,
		UserID: userID,
		Name:   storage.Name,
		Type:   storage.Type,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, userID string) ([]Storage, error) {
	query := `
		SELECT
		    tamiyo.storage.id AS id,
		    tamiyo.storage.name AS name,
		    tamiyo.storage.type AS type,
		    tamiyo.storage.added AS added,
			tamiyo.storage.updated as updated,
		    COUNT(tamiyo.cards.id) AS card_count
		FROM tamiyo.storage
		LEFT JOIN tamiyo.cards ON tamiyo.storage.id = tamiyo.cards.storage_id AND tamiyo.cards.user_id = $1
		WHERE tamiyo.storage.user_id = $1
		GROUP BY tamiyo.storage.id, tamiyo.storage.name, tamiyo.storage.type, tamiyo.storage.added, tamiyo.storage.updated
		ORDER BY tamiyo.storage.updated DESC
	`

	var rows []storageRow
	if err := r.db.SelectContext(ctx, &rows, query, userID); err != nil {
		return nil, err
	}

	storages := make([]Storage, 0, len(rows))
	for _, row := range rows {
		storages = append(storages, row.toDomain())
	}

	return storages, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, userID string, id int) (Storage, error) {
	query := `
		SELECT
		    tamiyo.storage.id AS id,
		    tamiyo.storage.name AS name,
		    tamiyo.storage.type AS type,
		    tamiyo.storage.added AS added,
			tamiyo.storage.updated as updated,
		    COUNT(tamiyo.cards.id) AS card_count
		FROM tamiyo.storage
		LEFT JOIN tamiyo.cards ON tamiyo.storage.id = tamiyo.cards.storage_id AND tamiyo.cards.user_id = $2
		WHERE tamiyo.storage.id = $1 AND tamiyo.storage.user_id = $2
		GROUP BY tamiyo.storage.id, tamiyo.storage.name, tamiyo.storage.type, tamiyo.storage.added, tamiyo.storage.updated
	`

	var row storageRow
	if err := r.db.GetContext(ctx, &row, query, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Storage{}, ErrNotFound
		}
		return Storage{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, storage Storage) (Storage, error) {
	row := toStorageRow(userID, storage)
	query := `
    	INSERT INTO tamiyo.storage (user_id, name, type)
     	VALUES (:user_id, :name, :type)
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

func (r *PostgresRepository) Update(ctx context.Context, userID string, s Storage) (Storage, error) {
	row := toStorageRow(userID, s)
	query := `
		UPDATE tamiyo.storage
		SET name = :name, type = :type
		WHERE id = :id AND user_id = :user_id
		RETURNING id, name, type, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Storage{}, err
	}
	defer stmt.Close()

	var updated storageRow
	if err := stmt.GetContext(ctx, &updated, row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Storage{}, ErrNotFound
		}
		return Storage{}, err
	}

	return updated.toDomain(), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, userID string, id int) error {
	query := `DELETE FROM tamiyo.storage WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, id, userID)
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
