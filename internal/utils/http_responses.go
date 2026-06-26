package utils

import (
	"net/http"

	"github.com/cedaesca/patient-finder/internal/pagination"
)

func HandleDataResponse(w http.ResponseWriter, statusCode int, data ResponseData) {
	WriteJSON(w, statusCode, Envelope{"data": data})
}

func HandleDataWithPaginationResponse(w http.ResponseWriter, statusCode int, data ResponseData, meta pagination.Metadata) {
	WriteJSON(w, statusCode, Envelope{
		"data":       data,
		"pagination": meta,
	})
}
