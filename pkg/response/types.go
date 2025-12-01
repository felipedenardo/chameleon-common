package response

type FieldError struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type Standard struct {
	Status      string       `json:"status"`
	Message     string       `json:"message"`
	Data        interface{}  `json:"data,omitempty"`
	Meta        interface{}  `json:"meta,omitempty"`
	FieldErrors []FieldError `json:"errors,omitempty"`
}
