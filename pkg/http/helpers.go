package http_api

import (
	"errors"
	"github.com/felipedenardo/chameleon-common/pkg/response"
	"github.com/felipedenardo/chameleon-common/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"log"
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

func RespondUnauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, response.NewErrorCustom(message))
}

func RespondDomainFail(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, response.NewFailCustom(message, nil))
}

func HandleInternalError(c *gin.Context, err error) {
	log.Printf("[SERVER ERROR] Unhandled error: %v", err)
	c.JSON(http.StatusInternalServerError, response.NewInternalErr())
}

func HandleBindingError(c *gin.Context, err error) {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		fieldErrors := validation.FromValidationErrors(ve)
		c.JSON(http.StatusBadRequest, response.NewFailCustom(response.MsgValidationErr, fieldErrors))
		return
	}

	c.JSON(http.StatusBadRequest, response.NewFailCustom(response.MsgInvalidJSON, []response.FieldError{{
		Field: "body",
		Error: err.Error(),
	}}))
}

func HandleParamError(c *gin.Context, field, message string) {
	c.JSON(http.StatusBadRequest, response.NewFailCustom(
		response.MsgParamErr,
		[]response.FieldError{{Field: field, Error: message}},
	))
}
