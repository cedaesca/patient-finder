package users

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/request"
)

func RequestUserIDRateLimitKey(r *http.Request) (string, bool) {
	requestUserID := request.GetUserID(r.Context())
	if requestUserID == uuid.Nil {
		return "", false
	}

	return requestUserID.String(), true
}
