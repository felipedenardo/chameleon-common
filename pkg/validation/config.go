package validation

import (
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func SetupCustomValidator() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
		if err := v.RegisterValidation("br_document", validateBRDocument); err != nil {
			panic(err)
		}
		if err := v.RegisterValidation("br_phone", validateBRPhone); err != nil {
			panic(err)
		}
		if err := v.RegisterValidation("br_zip", validateBRZip); err != nil {
			panic(err)
		}
	}
}
