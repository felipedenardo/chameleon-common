package response

func NewFieldError(field, message string) FieldError {
	return FieldError{
		Field: field,
		Error: message,
	}
}

func NewValidationErr(fieldErrors []FieldError) Standard {
	return Standard{
		Status:      "fail",
		Message:     MsgValidationErr,
		FieldErrors: fieldErrors,
	}
}

func NewInternalErr() Standard {
	return Standard{
		Status:  "error",
		Message: MsgInternalErr,
	}
}

func NewNotFound() Standard {
	return Standard{
		Status:  "error",
		Message: MsgNotFound,
	}
}

func NewFailCustom(message string, fieldErrors []FieldError) Standard {
	return Standard{
		Status:      "fail",
		Message:     message,
		FieldErrors: fieldErrors,
	}
}

func NewErrorCustom(message string) Standard {
	return Standard{
		Status:  "error",
		Message: message,
	}
}
