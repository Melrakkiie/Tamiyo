package apierr

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Mapping struct {
	Err     error
	Status  int
	Message string
}

func Respond(ctx *gin.Context, err error, mappings ...Mapping) {
	for _, m := range mappings {
		if errors.Is(err, m.Err) {
			message := m.Message
			if message == "" {
				message = err.Error()
			}
			ctx.JSON(m.Status, gin.H{"error": message})
			return
		}
	}

	_ = ctx.Error(err)
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
