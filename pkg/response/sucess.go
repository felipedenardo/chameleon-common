package response

import "math"

func NewSuccess(data interface{}) Standard {
	return Standard{
		Status:  MsgSuccess,
		Message: MsgSuccess,
		Data:    data,
	}
}

func NewCreated(data interface{}) Standard {
	return Standard{
		Status:  MsgSuccess,
		Message: MsgCreated,
		Data:    data,
	}
}

func NewPaged(data interface{}, page, perPage int, total int64) Standard {
	totalPages := 0
	if perPage > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}

	return Standard{
		Status:  MsgSuccess,
		Message: MsgDataFetched,
		Data:    data,
		Meta: PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

func NewUpdated(data interface{}) Standard {
	return NewSuccessCustom(MsgUpdated, data)
}

func NewDeleted() Standard {
	return NewSuccessCustom(MsgDeleted, nil)
}

func NewSuccessCustom(message string, data interface{}) Standard {
	return Standard{
		Status:  MsgSuccess,
		Message: message,
		Data:    data,
	}
}
