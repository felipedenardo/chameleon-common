package http_api

import (
	"errors"
	"github.com/felipedenardo/chameleon-common/pkg/response"
	"github.com/felipedenardo/chameleon-common/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"net/http"
)

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
