package deck

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
)

type folderService interface {
	GetFolders(ctx context.Context, userID string) ([]Folder, error)
	GetPublicFolders(ctx context.Context, ownerID string) ([]Folder, error)
	CreateFolder(ctx context.Context, userID string, name string, parentID *int) (Folder, error)
	UpdateFolder(ctx context.Context, userID string, id int, changes FolderChanges) (Folder, error)
	DeleteFolder(ctx context.Context, userID string, id int) error
	MoveDeckToFolder(ctx context.Context, userID string, deckID string, folderID *int) error
	SetFavorite(ctx context.Context, userID string, deckID string, favorite bool) error
}

type folderResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ParentID  *int   `json:"parent_id"`
	Collapsed bool   `json:"collapsed"`
	Added     string `json:"added"`
	Updated   string `json:"updated"`
}

func toFolderResponse(f Folder) folderResponse {
	return folderResponse{
		ID:        f.ID,
		Name:      f.Name,
		ParentID:  f.ParentID,
		Collapsed: f.Collapsed,
		Added:     f.Added.Format("2006-01-02 15:04:05"),
		Updated:   f.Updated.Format("2006-01-02 15:04:05"),
	}
}

type publicFolderResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID *int   `json:"parent_id"`
}

type createFolderRequest struct {
	Name     string `json:"name"`
	ParentID *int   `json:"parent_id"`
}

type updateFolderRequest struct {
	Name        *string `json:"name"`
	ParentID    *int    `json:"parent_id"`
	ClearParent bool    `json:"clear_parent"`
	Collapsed   *bool   `json:"collapsed"`
}

type moveDeckRequest struct {
	FolderID *int `json:"folder_id"`
}

func (h *Handler) registerFolderRoutes(router gin.IRoutes) {
	router.GET("/deck-folders", h.getFolders)
	router.POST("/deck-folders", h.createFolder)
	router.PATCH("/deck-folders/:id", h.updateFolder)
	router.DELETE("/deck-folders/:id", h.deleteFolder)
	router.GET("/users/:id/deck-folders", h.getUserPublicFolders)
	router.PUT("/deck/:id/folder", h.moveDeckToFolder)
	router.PUT("/deck/:id/favorite", h.favoriteDeck)
	router.DELETE("/deck/:id/favorite", h.unfavoriteDeck)
}

func respondFolderError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err,
		apierr.Mapping{Err: ErrNotFound, Status: http.StatusNotFound, Message: "deck not found"},
		apierr.Mapping{Err: ErrFolderNotFound, Status: http.StatusNotFound},
		apierr.Mapping{Err: ErrParentFolderNotFound, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrTargetFolderNotFound, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrFolderCycle, Status: http.StatusBadRequest},
		apierr.Mapping{Err: ErrInvalidFolderName, Status: http.StatusBadRequest},
	)
}

func validFolderID(id *int) bool {
	return id == nil || *id > 0
}

func (h *Handler) folderRequestContext(ctx *gin.Context) (string, int, bool) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", 0, false
	}
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", 0, false
	}
	return userID, id, true
}

func (h *Handler) getFolders(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	folders, err := h.service.GetFolders(ctx.Request.Context(), userID)
	if err != nil {
		respondFolderError(ctx, err)
		return
	}
	response := make([]folderResponse, 0, len(folders))
	for _, f := range folders {
		response = append(response, toFolderResponse(f))
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) getUserPublicFolders(ctx *gin.Context) {
	ownerID := strings.ToLower(ctx.Param("id"))
	if !userIDPattern.MatchString(ownerID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	folders, err := h.service.GetPublicFolders(ctx.Request.Context(), ownerID)
	if err != nil {
		respondFolderError(ctx, err)
		return
	}
	response := make([]publicFolderResponse, 0, len(folders))
	for _, f := range folders {
		response = append(response, publicFolderResponse{ID: f.ID, Name: f.Name, ParentID: f.ParentID})
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handler) createFolder(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req createFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !validFolderID(req.ParentID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrParentFolderNotFound.Error()})
		return
	}
	folder, err := h.service.CreateFolder(ctx.Request.Context(), userID, req.Name, req.ParentID)
	if err != nil {
		respondFolderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, toFolderResponse(folder))
}

func (h *Handler) updateFolder(ctx *gin.Context) {
	userID, id, ok := h.folderRequestContext(ctx)
	if !ok {
		return
	}
	var req updateFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !validFolderID(req.ParentID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrParentFolderNotFound.Error()})
		return
	}
	folder, err := h.service.UpdateFolder(ctx.Request.Context(), userID, id, FolderChanges(req))
	if err != nil {
		respondFolderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, toFolderResponse(folder))
}

func (h *Handler) deleteFolder(ctx *gin.Context) {
	userID, id, ok := h.folderRequestContext(ctx)
	if !ok {
		return
	}
	if err := h.service.DeleteFolder(ctx.Request.Context(), userID, id); err != nil {
		respondFolderError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) moveDeckToFolder(ctx *gin.Context) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}
	var req moveDeckRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if !validFolderID(req.FolderID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrTargetFolderNotFound.Error()})
		return
	}
	if err := h.service.MoveDeckToFolder(ctx.Request.Context(), userID, deckID, req.FolderID); err != nil {
		respondFolderError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handler) favoriteDeck(ctx *gin.Context) {
	h.setFavorite(ctx, true)
}

func (h *Handler) unfavoriteDeck(ctx *gin.Context) {
	h.setFavorite(ctx, false)
}

func (h *Handler) setFavorite(ctx *gin.Context, favorite bool) {
	userID, deckID, ok := h.deckRequestContext(ctx)
	if !ok {
		return
	}
	if err := h.service.SetFavorite(ctx.Request.Context(), userID, deckID, favorite); err != nil {
		respondFolderError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}
