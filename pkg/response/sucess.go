package response

import "math"

func NewSuccess(message string, data interface{}) Standard {
	return Standard{
		Status:  "success",
		Message: message,
		Data:    data,
	}
}

func NewPaged(message string, data interface{}, page, perPage int, total int64) Standard {
	totalPages := 0
	if perPage > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}

	return Standard{
		Status:  "success",
		Message: message,
		Data:    data,
		Meta: PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}
