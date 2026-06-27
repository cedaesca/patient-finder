package pagination

import (
	"math"
)

// Filters contiene los parámetros de paginación que vienen del request.
type Filters struct {
	Page     int
	PageSize int
}

func (f Filters) Limit() int {
	return f.PageSize
}

func (f Filters) Offset() int {
	return (f.Page - 1) * f.PageSize
}

type Metadata struct {
	CurrentPage  int  `json:"current_page,omitempty"`
	PageSize     int  `json:"page_size,omitempty"`
	FirstPage    int  `json:"first_page,omitempty"`
	LastPage     int  `json:"last_page,omitempty"`
	TotalRecords int  `json:"total_records,omitempty"`
	SearchTimeMs *int `json:"search_time_ms,omitempty"`
}

func CalculateMetadata(totalRecords, page, pageSize int) Metadata {
	if totalRecords == 0 {
		return Metadata{}
	}

	return Metadata{
		CurrentPage:  page,
		PageSize:     pageSize,
		FirstPage:    1,
		LastPage:     int(math.Ceil(float64(totalRecords) / float64(pageSize))),
		TotalRecords: totalRecords,
	}
}
