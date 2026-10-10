package deck

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type folderRow struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	ParentID  *int      `db:"parent_id"`
	Collapsed bool      `db:"collapsed"`
	Added     time.Time `db:"added"`
	Updated   time.Time `db:"updated"`
}

func (r folderRow) toDomain() Folder {
	return Folder(r)
}

func toFolders(rows []folderRow) []Folder {
	folders := make([]Folder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, row.toDomain())
	}
	return folders
}

const folderColumns = `id, name, parent_id, collapsed, added, updated`

func isForeignKeyViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23503"
}

func (r *PostgresRepository) FindFolders(ctx context.Context, userID string) ([]Folder, error) {
	var rows []folderRow
	if err := r.db.SelectContext(ctx, &rows, `
		SELECT `+folderColumns+` FROM tamiyo.deck_folders WHERE user_id = $1 ORDER BY lower(name), id
	`, userID); err != nil {
		return nil, fmt.Errorf("loading folders: %w", err)
	}
	return toFolders(rows), nil
}

func (r *PostgresRepository) FindPublicFolders(ctx context.Context, ownerID string) ([]Folder, error) {
	var rows []folderRow
	if err := r.db.SelectContext(ctx, &rows, `
		WITH RECURSIVE shown AS (
			SELECT f.id, f.parent_id FROM tamiyo.deck_folders f
			WHERE f.user_id = $1 AND EXISTS (
				SELECT 1 FROM tamiyo.deck d WHERE d.folder_id = f.id AND d.visibility = 'public'
			)
			UNION
			SELECT p.id, p.parent_id FROM tamiyo.deck_folders p JOIN shown ON p.id = shown.parent_id
		)
		SELECT id, name, parent_id, false AS collapsed, added, updated FROM tamiyo.deck_folders
		WHERE id IN (SELECT id FROM shown)
		ORDER BY lower(name), id
	`, ownerID); err != nil {
		return nil, fmt.Errorf("loading public folders: %w", err)
	}
	return toFolders(rows), nil
}

func (r *PostgresRepository) CreateFolder(ctx context.Context, userID string, f Folder) (Folder, error) {
	var row folderRow
	err := r.db.GetContext(ctx, &row, `
		INSERT INTO tamiyo.deck_folders (user_id, name, parent_id) VALUES ($1, $2, $3)
		RETURNING `+folderColumns, userID, f.Name, f.ParentID)
	if err != nil {
		if isForeignKeyViolation(err) {
			return Folder{}, ErrParentFolderNotFound
		}
		return Folder{}, fmt.Errorf("creating folder: %w", err)
	}
	return row.toDomain(), nil
}

func (r *PostgresRepository) UpdateFolder(ctx context.Context, userID string, f Folder) (Folder, error) {
	var row folderRow
	err := r.db.GetContext(ctx, &row, `
		UPDATE tamiyo.deck_folders SET name = $3, parent_id = $4, collapsed = $5
		WHERE id = $1 AND user_id = $2
		RETURNING `+folderColumns, f.ID, userID, f.Name, f.ParentID, f.Collapsed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Folder{}, ErrFolderNotFound
		}
		if isForeignKeyViolation(err) {
			return Folder{}, ErrParentFolderNotFound
		}
		return Folder{}, fmt.Errorf("updating folder: %w", err)
	}
	return row.toDomain(), nil
}

func (r *PostgresRepository) DeleteFolder(ctx context.Context, userID string, id int) (err error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var parentID *int
	if err = tx.GetContext(ctx, &parentID, `
		SELECT parent_id FROM tamiyo.deck_folders WHERE id = $1 AND user_id = $2 FOR UPDATE
	`, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrFolderNotFound
		}
		return fmt.Errorf("loading folder: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE tamiyo.deck SET folder_id = $3 WHERE folder_id = $1 AND user_id = $2
	`, id, userID, parentID); err != nil {
		return fmt.Errorf("moving the folder's decks: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE tamiyo.deck_folders SET parent_id = $3 WHERE parent_id = $1 AND user_id = $2
	`, id, userID, parentID); err != nil {
		return fmt.Errorf("moving the folder's subfolders: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM tamiyo.deck_folders WHERE id = $1 AND user_id = $2`, id, userID); err != nil {
		return fmt.Errorf("deleting folder: %w", err)
	}
	return tx.Commit()
}

func (r *PostgresRepository) SetDeckFolder(ctx context.Context, userID string, deckID string, folderID *int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tamiyo.deck SET folder_id = $3 WHERE id = $1 AND user_id = $2
	`, deckID, userID, folderID)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrTargetFolderNotFound
		}
		return fmt.Errorf("moving deck: %w", err)
	}
	return requireOneRow(result)
}

func (r *PostgresRepository) SetFavorite(ctx context.Context, userID string, deckID string, favorite bool) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE tamiyo.deck SET favorite = $3 WHERE id = $1 AND user_id = $2
	`, deckID, userID, favorite)
	if err != nil {
		return fmt.Errorf("updating favorite: %w", err)
	}
	return requireOneRow(result)
}

func requireOneRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
