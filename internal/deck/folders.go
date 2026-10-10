package deck

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrFolderNotFound       = errors.New("folder not found")
	ErrParentFolderNotFound = errors.New("parent_id does not reference one of your folders")
	ErrTargetFolderNotFound = errors.New("folder_id does not reference one of your folders")
	ErrFolderCycle          = errors.New("a folder cannot go inside itself or one of its subfolders")
	ErrInvalidFolderName    = errors.New("name must be between 1 and 100 characters")
)

const maxFolderNameLength = 100

type Folder struct {
	ID        int
	Name      string
	ParentID  *int
	Collapsed bool
	Added     time.Time
	Updated   time.Time
}

type FolderChanges struct {
	Name        *string
	ParentID    *int
	ClearParent bool
	Collapsed   *bool
}

type FolderRepository interface {
	FindFolders(ctx context.Context, userID string) ([]Folder, error)
	FindPublicFolders(ctx context.Context, ownerID string) ([]Folder, error)
	CreateFolder(ctx context.Context, userID string, f Folder) (Folder, error)
	UpdateFolder(ctx context.Context, userID string, f Folder) (Folder, error)
	DeleteFolder(ctx context.Context, userID string, id int) error
	SetDeckFolder(ctx context.Context, userID string, deckID string, folderID *int) error
	SetFavorite(ctx context.Context, userID string, deckID string, favorite bool) error
}

func cleanFolderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxFolderNameLength {
		return "", ErrInvalidFolderName
	}
	return name, nil
}

func (s *Service) GetFolders(ctx context.Context, userID string) ([]Folder, error) {
	return s.repo.FindFolders(ctx, userID)
}

func (s *Service) GetPublicFolders(ctx context.Context, ownerID string) ([]Folder, error) {
	return s.repo.FindPublicFolders(ctx, ownerID)
}

func (s *Service) CreateFolder(ctx context.Context, userID string, name string, parentID *int) (Folder, error) {
	cleaned, err := cleanFolderName(name)
	if err != nil {
		return Folder{}, err
	}
	return s.repo.CreateFolder(ctx, userID, Folder{Name: cleaned, ParentID: parentID})
}

func (s *Service) UpdateFolder(ctx context.Context, userID string, id int, changes FolderChanges) (Folder, error) {
	folders, err := s.repo.FindFolders(ctx, userID)
	if err != nil {
		return Folder{}, err
	}
	byID := make(map[int]Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	folder, ok := byID[id]
	if !ok {
		return Folder{}, ErrFolderNotFound
	}

	if changes.Name != nil {
		if folder.Name, err = cleanFolderName(*changes.Name); err != nil {
			return Folder{}, err
		}
	}
	if changes.Collapsed != nil {
		folder.Collapsed = *changes.Collapsed
	}
	if changes.ClearParent {
		folder.ParentID = nil
	} else if changes.ParentID != nil {
		if _, ok := byID[*changes.ParentID]; !ok {
			return Folder{}, ErrParentFolderNotFound
		}
		if insideFolder(byID, *changes.ParentID, id) {
			return Folder{}, ErrFolderCycle
		}
		parentID := *changes.ParentID
		folder.ParentID = &parentID
	}
	return s.repo.UpdateFolder(ctx, userID, folder)
}

func insideFolder(byID map[int]Folder, id int, ancestorID int) bool {
	seen := make(map[int]bool)
	for current := &id; current != nil && !seen[*current]; current = byID[*current].ParentID {
		if *current == ancestorID {
			return true
		}
		seen[*current] = true
	}
	return false
}

func (s *Service) DeleteFolder(ctx context.Context, userID string, id int) error {
	return s.repo.DeleteFolder(ctx, userID, id)
}

func (s *Service) MoveDeckToFolder(ctx context.Context, userID string, deckID string, folderID *int) error {
	return s.repo.SetDeckFolder(ctx, userID, deckID, folderID)
}

func (s *Service) SetFavorite(ctx context.Context, userID string, deckID string, favorite bool) error {
	return s.repo.SetFavorite(ctx, userID, deckID, favorite)
}
