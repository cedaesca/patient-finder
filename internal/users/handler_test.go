package users

import (
	"bytes"
	"context"
	"errors"

	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/testutil"
	"github.com/stretchr/testify/require"
)

type usersServiceMock struct {
	getUserByIDFn                func(ctx context.Context, id uuid.UUID) (*User, error)
	updateUserFn                 func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error)
	startLoggedInUserPasswordOtp func(ctx context.Context, id uuid.UUID) error
	updateLoggedInUserPasswordFn func(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error
	getUserIDByEmailFn           func(ctx context.Context, email string) (uuid.UUID, error)
	createUserFn                 func(ctx context.Context, input CreateUserInput, actorID uuid.UUID) (*User, error)
	listUsersFn                  func(ctx context.Context, filters pagination.Filters) ([]User, pagination.Metadata, error)
	adminUpdateUserFn            func(ctx context.Context, id uuid.UUID, input AdminUpdateUserInput, actorID uuid.UUID) (*User, error)
	deleteUserFn                 func(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error
}

func (m *usersServiceMock) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}

	return nil, nil
}

func (m *usersServiceMock) UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	if m.updateUserFn != nil {
		return m.updateUserFn(ctx, id, input)
	}

	return nil, nil
}

func (m *usersServiceMock) StartLoggedInUserPasswordOtp(ctx context.Context, id uuid.UUID) error {
	if m.startLoggedInUserPasswordOtp != nil {
		return m.startLoggedInUserPasswordOtp(ctx, id)
	}

	return nil
}

func (m *usersServiceMock) UpdateLoggedInUserPassword(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
	if m.updateLoggedInUserPasswordFn != nil {
		return m.updateLoggedInUserPasswordFn(ctx, id, input)
	}

	return nil
}

func (m *usersServiceMock) GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	if m.getUserIDByEmailFn != nil {
		return m.getUserIDByEmailFn(ctx, email)
	}

	return uuid.Nil, nil
}

func (m *usersServiceMock) CreateUser(ctx context.Context, input CreateUserInput, actorID uuid.UUID) (*User, error) {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, input, actorID)
	}
	return nil, nil
}

func (m *usersServiceMock) ListUsers(ctx context.Context, filters pagination.Filters) ([]User, pagination.Metadata, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(ctx, filters)
	}
	return nil, pagination.Metadata{}, nil
}

func (m *usersServiceMock) AdminUpdateUser(ctx context.Context, id uuid.UUID, input AdminUpdateUserInput, actorID uuid.UUID) (*User, error) {
	if m.adminUpdateUserFn != nil {
		return m.adminUpdateUserFn(ctx, id, input, actorID)
	}
	return nil, nil
}

func (m *usersServiceMock) DeleteUser(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	if m.deleteUserFn != nil {
		return m.deleteUserFn(ctx, id, actorID)
	}
	return nil
}

func newTestUsersHandler(service UsersService) *Handler {
	return NewHandler(service)
}

func TestUsersHandler_HandleGetMe(t *testing.T) {
	t.Run("returns internal server error when user is missing from context", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{})

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)

		h.HandleGetMe(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "internal server error", body["message"])
	})

	t.Run("returns internal server error on unexpected service error", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return nil, errors.New("boom")
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandleGetMe(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "internal server error", body["message"])
	})

	t.Run("returns authenticated user", func(t *testing.T) {
		userID := uuid.New()
		h := newTestUsersHandler(&usersServiceMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				require.Equal(t, userID, id)
				return &User{
					ID:       userID,
					Name:     "Lupi",
					LastName: "Tester",
					Email:    "lupi@example.com",
				}, nil
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/users/me", nil)
		req = req.WithContext(request.SetUserID(ctx, userID))

		h.HandleGetMe(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)

		userField, ok := dataField["user"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, userID.String(), userField["id"])
		require.Equal(t, "lupi@example.com", userField["email"])
		require.Equal(t, "Lupi", userField["name"])
	})
}

func TestUsersHandler_HandlePatchMe(t *testing.T) {
	t.Run("returns bad request for invalid json", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/users/me", bytes.NewBufferString("{"))
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("returns validation error for short name", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPatch, "/users/me", map[string]string{
			"name": "ab",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])

		errorsField, ok := body["errors"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "name must be at least 3 characters", errorsField["name"])
	})

	t.Run("returns not found when user does not exist", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			updateUserFn: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				return nil, contracts.ErrNotFound
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPatch, "/users/me", map[string]string{
			"name": "Lupita",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "user not found", body["message"])
	})

	t.Run("returns conflict for duplicate name data", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			updateUserFn: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				return nil, ErrDuplicateName
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPatch, "/users/me", map[string]string{
			"name": "Lupita",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "the provided name data is already taken", body["message"])
	})

	t.Run("returns internal server error on unexpected service error", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			updateUserFn: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				return nil, errors.New("boom")
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPatch, "/users/me", map[string]string{
			"name": "Lupita",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "internal server error", body["message"])
	})

	t.Run("updates only provided fields", func(t *testing.T) {
		userID := uuid.New()
		h := newTestUsersHandler(&usersServiceMock{
			updateUserFn: func(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
				require.Equal(t, userID, id)
				require.NotNil(t, input.Name)
				require.Equal(t, "Lupita", *input.Name)
				require.Nil(t, input.LastName)

				return &User{
					ID:       userID,
					Name:     "Lupita",
					LastName: "Original",
					Email:    "lupi@example.com",
				}, nil
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPatch, "/users/me", map[string]string{
			"name": "Lupita",
		})
		req = req.WithContext(request.SetUserID(ctx, userID))

		h.HandlePatchMe(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)

		userField, ok := dataField["user"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, userID.String(), userField["id"])
		require.Equal(t, "Lupita", userField["name"])
		require.Equal(t, "Original", userField["last_name"])
	})
}

func TestUsersHandler_HandleCreatePasswordOtp(t *testing.T) {
	t.Run("returns accepted on success", func(t *testing.T) {
		userID := uuid.New()
		h := newTestUsersHandler(&usersServiceMock{
			startLoggedInUserPasswordOtp: func(ctx context.Context, id uuid.UUID) error {
				require.Equal(t, userID, id)
				return nil
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users/me/password/otp", nil)
		req = req.WithContext(request.SetUserID(ctx, userID))

		h.HandleCreatePasswordOtp(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code)
	})

	t.Run("returns internal server error on service failure", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			startLoggedInUserPasswordOtp: func(ctx context.Context, id uuid.UUID) error {
				return errors.New("boom")
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/users/me/password/otp", nil)
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandleCreatePasswordOtp(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestUsersHandler_HandleUpdatePassword(t *testing.T) {
	t.Run("returns validation errors", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/users/me/password", map[string]string{
			"current_password": "short",
			"new_password":     "short",
			"otp":              "",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandleUpdatePassword(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("returns forbidden for wrong current password", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			updateLoggedInUserPasswordFn: func(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
				return ErrInvalidCurrentPassword
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/users/me/password", map[string]string{
			"current_password": "currentPass123",
			"new_password":     "newPass123",
			"otp":              "ABC123",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandleUpdatePassword(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("returns forbidden for invalid otp", func(t *testing.T) {
		h := newTestUsersHandler(&usersServiceMock{
			updateLoggedInUserPasswordFn: func(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
				return ErrInvalidPasswordChangeOtp
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/users/me/password", map[string]string{
			"current_password": "currentPass123",
			"new_password":     "newPass123",
			"otp":              "BADOTP",
		})
		req = req.WithContext(request.SetUserID(ctx, uuid.New()))

		h.HandleUpdatePassword(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid otp", body["message"])
	})

	t.Run("returns ok when password changes", func(t *testing.T) {
		userID := uuid.New()
		h := newTestUsersHandler(&usersServiceMock{
			updateLoggedInUserPasswordFn: func(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
				require.Equal(t, userID, id)
				require.Equal(t, "currentPass123", input.CurrentPassword)
				require.Equal(t, "newPass123", input.NewPassword)
				require.Equal(t, "ABC123", input.Otp)
				return nil
			},
		})

		ctx := context.Background()

		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/users/me/password", map[string]string{
			"current_password": "currentPass123",
			"new_password":     "newPass123",
			"otp":              "ABC123",
		})
		req = req.WithContext(request.SetUserID(ctx, userID))

		h.HandleUpdatePassword(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
	})
}
