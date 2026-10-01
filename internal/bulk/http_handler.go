package bulk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"Melrakkiie/Tamiyo/internal/auth"
)

type importService interface {
	ImportManaBox(ctx context.Context, userID string, r io.Reader) (Summary, error)
	ImportMoxfieldCollection(ctx context.Context, userID string, storageID int, r io.Reader) (Summary, error)
	ImportMoxfieldDeck(ctx context.Context, userID string, req MoxfieldDeckImportRequest, r io.Reader) (Summary, error)

	ExportManaBox(ctx context.Context, userID string, w io.Writer) error
	ExportMoxfieldCollection(ctx context.Context, userID string, w io.Writer) error
	ExportMoxfieldDeck(ctx context.Context, userID string, deckID int, w io.Writer) error
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
	router.POST("/import/moxfield/deck", h.importMoxfieldDeck)

	router.GET("/export/manabox", h.exportManaBox)
	router.GET("/export/moxfield/collection", h.exportMoxfieldCollection)
	router.GET("/export/moxfield/deck/:id", h.exportMoxfieldDeck)
}

func (h *Handler) importManaBox(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
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

	summary, err := h.service.ImportManaBox(ctx.Request.Context(), userID, file)
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

func (h *Handler) importMoxfieldDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	name := ctx.PostForm("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	format := ctx.PostForm("format")
	if format == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "format is required"})
		return
	}

	req := MoxfieldDeckImportRequest{
		Name:                   name,
		Format:                 format,
		CommanderFromFirstLine: true,
	}

	if raw := ctx.PostForm("commander_from_first_line"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "commander_from_first_line must be a boolean"})
			return
		}
		req.CommanderFromFirstLine = parsed
	}

	if raw := ctx.PostForm("storage_id"); raw != "" {
		storageID, err := strconv.Atoi(raw)
		if err != nil || storageID < 1 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id must be a positive integer"})
			return
		}
		req.StorageID = &storageID
	}

	file, err := openUploadedFile(ctx, "file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	summary, err := h.service.ImportMoxfieldDeck(ctx.Request.Context(), userID, req, file)
	h.respondImport(ctx, summary, err)
}

func (h *Handler) exportManaBox(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	var buf bytes.Buffer
	if err := h.service.ExportManaBox(ctx.Request.Context(), userID, &buf); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	var buf bytes.Buffer
	if err := h.service.ExportMoxfieldCollection(ctx.Request.Context(), userID, &buf); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Header("Content-Disposition", `attachment; filename="Moxfield_Collection_export.csv"`)
	ctx.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

func (h *Handler) exportMoxfieldDeck(ctx *gin.Context) {
	userID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	deckID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var buf bytes.Buffer
	if err := h.service.ExportMoxfieldDeck(ctx.Request.Context(), userID, deckID, &buf); err != nil {
		if errors.Is(err, ErrDeckNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "deck not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Header("Content-Disposition", `attachment; filename="Moxfield_Deck_export.txt"`)
	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", buf.Bytes())
}

func (h *Handler) respondImport(ctx *gin.Context, summary Summary, err error) {
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidFile):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, ErrTargetStorageNotFound):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "storage_id does not reference an existing storage"})
		case errors.Is(err, ErrScryfallUnavailable):
			ctx.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.IndentedJSON(http.StatusOK, summary)
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
	errMissingFile    = errors.New("a file is required (multipart field \"file\")")
	errUnreadableFile = errors.New("could not read the uploaded file")
)
