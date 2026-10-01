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
		return "é obrigatório"
	case "email":
		return "precisa ser um e-mail válido"
	case "min":
		return "precisa ter pelo menos " + fe.Param() + " caracteres"
	case "max":
		return "pode ter no máximo " + fe.Param() + " caracteres"
	case "len":
		return "precisa ter exatamente " + fe.Param() + " caracteres"
	case "eqfield":
		return "não confere com " + strings.ToLower(fe.Param())
	case "oneof":
		return "precisa ser um destes: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	case "uuid":
		return "identificador inválido"
	case "url":
		return "precisa ser um endereço válido"
	case "br_document":
		return "precisa ser um CPF ou CNPJ válido"
	case "br_phone":
		return "precisa ser um telefone válido"
	case "br_zip":
		return "precisa ser um CEP válido"
	case "birth_date":
		return "precisa ser uma data de nascimento válida"
	case "numeric":
		return "precisa ser um número"
	case "alphanum":
		return "só pode ter letras e números"
	case "gte":
		return "precisa ser maior ou igual a " + fe.Param()
	case "lte":
		return "precisa ser menor ou igual a " + fe.Param()
	default:
		return "é inválido"
	}
}
