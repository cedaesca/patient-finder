package request

import (
	"net/http"
	"strconv"
)

func ReadIntQueryParam(r *http.Request, key string, defaultValue int) int {
	s := r.URL.Query().Get(key)

	if s == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}

	return value
}
