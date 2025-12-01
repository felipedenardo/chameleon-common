package response

func NewFieldError(field, message string) FieldError {
	return FieldError{
		Field: field,
		Error: message,
	}
}

func NewFail(message string, fieldErrors []FieldError) Standard {
	return Standard{
		Status:      "fail",
		Message:     message,
		FieldErrors: fieldErrors,
	}
}

func NewError(message string) Standard {
	return Standard{
		Status:  "error",
		Message: message,
	}
}

func NewNotFound(message string) Standard {
	return Standard{
		Status:  "error",
		Message: message,
	}
}
