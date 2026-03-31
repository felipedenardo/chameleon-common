package http

import (
	"context"
	"errors"
	"github.com/felipedenardo/chameleon-common/pkg/response"
	"github.com/felipedenardo/chameleon-common/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	zlog "github.com/rs/zerolog/log"
	"net/http"
)

func RespondCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, response.NewCreated(data))
}

func RespondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, response.NewSuccess(data))
}

func RespondUpdated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, response.NewUpdated(data))
}

func RespondDeleted(c *gin.Context) {
	c.JSON(http.StatusOK, response.NewDeleted())
}

func RespondPaged(c *gin.Context, data interface{}, page, perPage int, total int64) {
	c.JSON(http.StatusOK, response.NewPaged(data, page, perPage, total))
}

func RespondValidation(c *gin.Context, errs []response.FieldError) {
	c.JSON(http.StatusBadRequest, response.NewValidationErr(errs))
}

func RespondNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, response.NewNotFound())
}

func RespondNoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, response.NewNotFound())
}

func RespondTimeout(c *gin.Context) {
	c.JSON(http.StatusRequestTimeout, response.NewNotFound())
}

func RespondUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, response.NewErrorCustom(message))
}

func RespondForbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, response.NewErrorCustom(message))
}

func RespondDomainFail(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, response.NewFailCustom(message, nil))
}

func RespondClientCancelled(c *gin.Context) {
	zlog.Info().Msg("Client closed the connection")
	c.JSON(499, response.NewFailCustom("A requisição foi cancelada pelo usuário.", nil))
}

func RespondInternalError(c *gin.Context, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		zlog.Warn().Err(err).Msg("Operation timed out")
		RespondTimeout(c)
		return
	}

	if errors.Is(err, context.Canceled) {
		zlog.Info().Msg("Client cancelled the request")
		RespondClientCancelled(c)
		return
	}

	zlog.Error().Err(err).Msg("Unhandled server error")
	c.JSON(http.StatusInternalServerError, response.NewInternalErr())
}

func RespondBindingError(c *gin.Context, err error) {
	var ve validator.ValidationErrors

	if errors.As(err, &ve) {
		fieldErrors := validation.FromValidationErrors(ve)
		RespondValidation(c, fieldErrors)
		return
	}

	c.JSON(http.StatusBadRequest, response.NewFailCustom(response.MsgInvalidJSON,
		[]response.FieldError{
			response.NewFieldError("body", err.Error()),
		},
	))
}

func RespondParamError(c *gin.Context, field, message string) {
	c.JSON(http.StatusBadRequest, response.NewFailCustom(
		response.MsgParamErr,
		[]response.FieldError{
			response.NewFieldError(field, message),
		},
	))
}
