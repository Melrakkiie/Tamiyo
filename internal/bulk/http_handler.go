package bulk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/apierr"
	"Melrakkiie/Tamiyo/internal/auth"
	"Melrakkiie/Tamiyo/internal/deck"
)

type importService interface {
	ImportManaBox(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error)
	ImportMoxfieldCollection(ctx context.Context, userID string, storageID int, r io.Reader) (Summary, error)
	ImportCardList(ctx context.Context, userID string, storageID *int, r io.Reader) (Summary, error)
	ImportIntoDeck(ctx context.Context, userID string, deckID string, commanderFromFirstLine bool, r io.Reader) (Summary, error)

	ExportManaBox(ctx context.Context, userID string, storageID *int, w io.Writer) error
	ExportMoxfieldCollection(ctx context.Context, userID string, storageID *int, w io.Writer) error
	ExportDeck(ctx context.Context, userID string, deckID string, format string, w io.Writer) error

	RefreshCardDetails(ctx context.Context, userID string, afterID int) (DetailsRefreshSummary, error)
	CommitPendingCards(ctx context.Context, userID string, deckID string, storageID, pendingID, quantity *int) (PendingCommitSummary, error)
}

type Handler struct {
	service importService
}

func NewHandler(service importService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(router gin.IRoutes) {
	router.POST("/import/manabox", h.importManaBox)
	router.POST("/import/moxfield/collection", h.importMoxfieldCollection)
	router.POST("/import/list", h.importCardList)

	router.GET("/export/manabox", h.exportManaBox)
	router.GET("/export/moxfield/collection", h.exportMoxfieldCollection)

	router.POST("/cards/refresh-details", h.refreshCardDetails)
	router.POST("/deck/:id/pending/commit", h.commitPendingCards)
	router.POST("/deck/:id/import", h.importIntoDeck)
	router.GET("/deck/:id/export", h.exportDeck)
}

func (h *Handler) refreshCardDetails(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	afterID := 0
	if raw := ctx.Query("after_id"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "after_id must be a non-negative integer"})
			return
		}
		afterID = parsed
	}

	summary, err := h.service.RefreshCardDetails(ctx.Request.Context(), userID, afterID)
	if err != nil {
		apierr.Respond(ctx, err, apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway})
		return
	}

	ctx.JSON(http.StatusOK, summary)
}

func (h *Handler) importManaBox(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	storageID, valid := optionalStorageID(ctx.PostForm("storage_id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": errInvalidStorageID.Error()})
		return
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.ImportManaBox(ctx.Request.Context(), userID, storageID, file)
	h.respondImport(ctx, summary, err)
}

func (h *Handler) importMoxfieldCollection(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	storageID, err := strconv.Atoi(ctx.PostForm("storage_id"))
	if err != nil || storageID < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id is required and must be a positive integer"})
		return
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.ImportMoxfieldCollection(ctx.Request.Context(), userID, storageID, file)
	h.respondImport(ctx, summary, err)
}

func (h *Handler) importCardList(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	storageID, valid := optionalStorageID(ctx.PostForm("storage_id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": errInvalidStorageID.Error()})
		return
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.ImportCardList(ctx.Request.Context(), userID, storageID, file)
	h.respondImport(ctx, summary, err)
}

func (h *Handler) importIntoDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	commanderFromFirstLine := false
	if raw := ctx.PostForm("commander_from_first_line"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "commander_from_first_line must be a boolean"})
			return
		}
		commanderFromFirstLine = parsed
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.ImportIntoDeck(ctx.Request.Context(), userID, deckID, commanderFromFirstLine, file)
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrInvalidFile, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway},
		)
		return
	}

	ctx.IndentedJSON(http.StatusOK, summary)
}

func (h *Handler) exportManaBox(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	storageID, valid := optionalStorageID(ctx.Query("storage_id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": errInvalidStorageID.Error()})
		return
	}

	var buf bytes.Buffer
	if err := h.service.ExportManaBox(ctx.Request.Context(), userID, storageID, &buf); err != nil {
		respondExportError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", `attachment; filename="ManaBox_Collection_export.csv"`)
	ctx.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (h *Handler) exportMoxfieldCollection(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	storageID, valid := optionalStorageID(ctx.Query("storage_id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": errInvalidStorageID.Error()})
		return
	}

	var buf bytes.Buffer
	if err := h.service.ExportMoxfieldCollection(ctx.Request.Context(), userID, storageID, &buf); err != nil {
		respondExportError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", `attachment; filename="Moxfield_Collection_export.csv"`)
	ctx.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

var deckExportFilenames = map[string]string{
	DeckExportMoxfield: "Deck_moxfield.txt",
	DeckExportPlain:    "Deck_list.txt",
	DeckExportArena:    "Deck_arena.txt",
}

func (h *Handler) exportDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, valid := deck.ParseID(ctx.Param("id"))
	if !valid {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	format := ctx.DefaultQuery("format", DeckExportMoxfield)
	filename, known := deckExportFilenames[format]
	if !known {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": ErrUnknownExportFormat.Error()})
		return
	}

	var buf bytes.Buffer
	if err := h.service.ExportDeck(ctx.Request.Context(), userID, deckID, format, &buf); err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrDeckNotFound, Status: http.StatusNotFound, Message: "deck not found"},
			apierr.Mapping{Err: ErrUnknownExportFormat, Status: http.StatusBadRequest},
		)
		return
	}

	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", buf.Bytes())
}

func (h *Handler) respondImport(ctx *gin.Context, summary Summary, err error) {
	if err != nil {
		apierr.Respond(ctx, err,
			apierr.Mapping{Err: ErrInvalidFile, Status: http.StatusBadRequest},
			apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusBadRequest, Message: "storage_id does not reference an existing storage"},
			apierr.Mapping{Err: ErrScryfallUnavailable, Status: http.StatusBadGateway},
		)
		return
	}

	ctx.IndentedJSON(http.StatusOK, summary)
}

func respondExportError(ctx *gin.Context, err error) {
	apierr.Respond(ctx, err, apierr.Mapping{Err: ErrTargetStorageNotFound, Status: http.StatusNotFound, Message: "storage not found"})
}

func optionalStorageID(raw string) (*int, bool) {
	if raw == "" {
		return nil, true
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id < 1 {
		return nil, false
	}
	return &id, true
}

func openUploadedFile(ctx *gin.Context, field string) (multipartFile, error) {
	header, err := ctx.FormFile(field)
	if err != nil {
		return nil, errMissingFile
	}
	f, err := header.Open()
	if err != nil {
		return nil, errUnreadableFile
	}
	return f, nil
}

type multipartFile interface {
	io.Reader
	io.Closer
}

var (
	errMissingFile      = errors.New("a file is required (multipart field \"file\")")
	errUnreadableFile   = errors.New("could not read the uploaded file")
	errInvalidStorageID = errors.New("storage_id must be a positive integer")
)
