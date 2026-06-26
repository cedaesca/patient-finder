package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Envelope map[string]interface{}

// Data represents the inner payload for successful responses.
type ResponseData = map[string]interface{}

func WriteJSON(w http.ResponseWriter, status int, data Envelope) error {
	js, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return err
	}

	js = append(js, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)
	return nil
}

func ReadIDParam(r *http.Request, key string) (string, error) {
	paramKey := "id"

	if key != "" {
		paramKey = key
	}

	idParam := chi.URLParam(r, paramKey)

	const invalidMsg = "invalid id parameter"

	if idParam == "" {
		return "", errors.New(invalidMsg)
	}

	if _, err := uuid.Parse(idParam); err != nil {
		return "", errors.New(invalidMsg)
	}

	return idParam, nil
}

func IsValidTimezone(tz string) bool {
	_, err := time.LoadLocation(tz)
	return err == nil
}
