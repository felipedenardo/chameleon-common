package validation

import (
	"errors"
	"github.com/felipedenardo/chameleon-common/pkg/response"
	"github.com/go-playground/validator/v10"
	"reflect"
	"strings"
)

var validate *validator.Validate

func init() {
	validate = validator.New()

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
}

func ValidateRequest(s interface{}) []response.FieldError {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) && len(ve) > 0 {
		return FromValidationErrors(ve)
	}

	return []response.FieldError{{
		Field: "payload",
		Error: "invalid input data format",
	}}
}

func FromValidationErrors(ve validator.ValidationErrors) []response.FieldError {
	var errs []response.FieldError
	for _, err := range ve {
		errs = append(errs, response.FieldError{
			Field: err.Field(),
			Error: validationErrorMessage(err),
		})
	}
	return errs
}

func validationErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + fe.Param() + " characters long"
	case "max":
		return "must be at most " + fe.Param() + " characters long"
	case "len":
		return "must be exactly " + fe.Param() + " characters long"
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	case "uuid":
		return "must be a valid UUID"
	case "url":
		return "must be a valid URL"
	default:
		return "is invalid"
	}
}
