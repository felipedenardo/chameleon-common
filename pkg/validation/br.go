package validation

import (
	"reflect"
	"strings"
	"unicode"

	validator "github.com/go-playground/validator/v10"
)

func validateBRDocument(fl validator.FieldLevel) bool {
	value, ok := stringValue(fl.Field())
	if !ok {
		return false
	}
	value = digitsOnly(value)
	if value == "" {
		return true
	}
	switch len(value) {
	case 11:
		return isValidCPF(value)
	case 14:
		return isValidCNPJ(value)
	default:
		return false
	}
}

func validateBRPhone(fl validator.FieldLevel) bool {
	value, ok := stringValue(fl.Field())
	if !ok {
		return false
	}
	value = digitsOnly(value)
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "55") && (len(value) == 12 || len(value) == 13) {
		value = value[2:]
	}
	if len(value) != 10 && len(value) != 11 {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func validateBRZip(fl validator.FieldLevel) bool {
	value, ok := stringValue(fl.Field())
	if !ok {
		return false
	}
	value = digitsOnly(value)
	return len(value) == 8
}

func stringValue(field reflect.Value) (string, bool) {
	if field.Kind() != reflect.String {
		return "", false
	}
	return field.String(), true
}

func digitsOnly(value string) string {
	if value == "" {
		return ""
	}
	b := strings.Builder{}
	b.Grow(len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allDigitsEqual(value string) bool {
	if len(value) == 0 {
		return true
	}
	first := value[0]
	for i := 1; i < len(value); i++ {
		if value[i] != first {
			return false
		}
	}
	return true
}

func isValidCPF(value string) bool {
	if len(value) != 11 || allDigitsEqual(value) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(value[i]-'0') * (10 - i)
	}
	d1 := 11 - (sum % 11)
	if d1 >= 10 {
		d1 = 0
	}
	if int(value[9]-'0') != d1 {
		return false
	}
	sum = 0
	for i := 0; i < 10; i++ {
		sum += int(value[i]-'0') * (11 - i)
	}
	d2 := 11 - (sum % 11)
	if d2 >= 10 {
		d2 = 0
	}
	return int(value[10]-'0') == d2
}

func isValidCNPJ(value string) bool {
	if len(value) != 14 || allDigitsEqual(value) {
		return false
	}
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	calc := func(input string, weights []int) int {
		sum := 0
		for i := 0; i < len(weights); i++ {
			sum += int(input[i]-'0') * weights[i]
		}
		rest := sum % 11
		if rest < 2 {
			return 0
		}
		return 11 - rest
	}
	d1 := calc(value[:12], weights1)
	if int(value[12]-'0') != d1 {
		return false
	}
	d2 := calc(value[:13], weights2)
	return int(value[13]-'0') == d2
}
