package handler

import (
	"context"
	"encoding/json"
	"errors"
	"lensamity/internal/auth"
	"lensamity/internal/middleware"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/google/uuid"
)

type authService interface {
	Signup(context.Context, string, string, string) (*auth.SignupResponse, error)
	Login(context.Context, string, string) (*auth.LoginResult, error)
	Logout(context.Context, string) error
	LogoutAll(context.Context, uuid.UUID) error
	UpdatePassword(context.Context, auth.UpdatePasswordParams) (*auth.UpdatePasswordResult, error)
	SessionOwner(ctx context.Context, userID uuid.UUID) (*auth.SessionOwnerResult, error)
	UsernameExists(ctx context.Context, username string) (bool, error)
}

type AuthHandler struct {
	authService authService
}

func NewAuthHandler(service authService) (*AuthHandler, error) {
	if service == nil {
		return nil, errors.New("handler: nil auth service")
	}
	v := reflect.ValueOf(service)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil, errors.New("handler: nil auth service")
	}

	return &AuthHandler{
		authService: service,
	}, nil
}

type SignupRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type SignupResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func (h *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

	var req SignupRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON")
		return
	}

	if req.Username == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "invalid_signup", "username and password are required")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	user, err := h.authService.Signup(ctx, req.Username, req.DisplayName, req.Password)

	if err != nil {
		if errors.Is(err, auth.ErrUsernameUnavailable) {
			WriteError(w, http.StatusConflict, "username_taken", "username is not available")
			return
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, auth.ErrInternal) {
			slog.Error("Signup: request failed", "error", err)
			WriteError(w, statusForAuthError(err), "internal_error", "something went wrong")
			return
		}
		WriteError(w, http.StatusBadRequest, "invalid_signup", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(SignupResponse{Username: user.UsernameKey, DisplayName: user.UsernameDisplay})

	if err != nil {
		slog.Error("Signup: failed to encode response", "error", err)
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

	var req LoginRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON")
		return
	}

	if req.Username == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "invalid_request", "username and password are required")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	user, err := h.authService.Login(ctx, req.Username, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "")
			return
		}
		slog.Error("Login request failed", "error", err)
		WriteError(w, statusForAuthError(err), "internal_error", "something went wrong")
		return
	}

	middleware.SetSessionCookie(w, user.CookieToken, user.CookieExpiredAt)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(LoginResponse{
		Username:    user.Username,
		DisplayName: user.DisplayName,
	})

	if err != nil {
		slog.Error("Login: failed to encode response", "error", err)
	}
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cookie, err := r.Cookie(middleware.SessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		middleware.ClearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		middleware.ClearSessionCookie(w)
		WriteError(w, http.StatusBadRequest, "invalid_cookie", "invalid session cookie")
		return
	}

	err = h.authService.Logout(ctx, cookie.Value)
	if err != nil {
		slog.Error("Logout request failed", "error", err)
		WriteError(w, statusForAuthError(err), "internal_error", "something went wrong")
		return
	}

	middleware.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := h.authService.LogoutAll(ctx, userID)
	if err != nil {
		slog.Error("LogoutAll request failed", "error", err)
		WriteError(w, statusForAuthError(err), "internal_error", "something went wrong")
		return
	}

	middleware.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type UpdateMyPasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
	RevokeAll   bool   `json:"revokeAll"`
}

func (h *AuthHandler) UpdateMyPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

	var req UpdateMyPasswordRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON")
		return
	}

	if req.NewPassword == "" || req.OldPassword == "" {
		WriteError(w, http.StatusBadRequest, "malformed_json", "missing field")
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	username, ok := r.Context().Value(middleware.UsernameKey).(string)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err != nil {
		middleware.ClearSessionCookie(w)
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.authService.UpdatePassword(ctx, auth.UpdatePasswordParams{
		UserID:              userID,
		Username:            username,
		CurrentPassword:     req.OldPassword,
		NewPassword:         req.NewPassword,
		CurrentSessionToken: cookie.Value,
		RevokeAll:           req.RevokeAll,
	})
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusBadRequest, "invalid_old_password", "current password is incorrect. Check for typos and try again.")
			return
		}
		if errors.Is(err, auth.ErrNewPasswordValidation) {
			WriteError(w, http.StatusBadRequest, "invalid_new_password", err.Error())
			return
		}
		if errors.Is(err, auth.ErrPasswordChanged) {
			WriteError(w, http.StatusConflict, "invalid_new_password", "password was changed in another session. Enter your current password and try again.")
			return
		}
		if errors.Is(err, auth.ErrInvalidSession) {
			middleware.ClearSessionCookie(w)
			WriteError(w, http.StatusUnauthorized, "unauthorized", "")
			return
		}
		slog.Error("UpdatePassword: request failed", "error", err)
		WriteError(w, statusForAuthError(err), "internal_error", "something went wrong")
		return
	}

	middleware.SetSessionCookie(w, result.CookieToken, result.CookieExpiredAt)
	w.WriteHeader(http.StatusNoContent)
}

type SessionResponse struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	user, err := h.authService.SessionOwner(ctx, userID)
	if err != nil {
		slog.Error("Session request failed", "error", err)
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(SessionResponse{Username: user.UsernameKey, DisplayName: user.UsernameDisplay})

	if err != nil {
		slog.Error("Session: failed to encode response", "error", err)
	}
}

type UsernameAvailabilityResponse struct {
	Available       bool   `json:"isAvailable"`
	ValidationError string `json:"validationError,omitempty"`
}

func (h *AuthHandler) UsernameAvailability(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	var username string
	if username = query.Get("username"); username == "" {
		WriteError(w, http.StatusBadRequest, "invalid_params", "username parameter is required")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	isAvailable, err := h.authService.UsernameExists(ctx, username)
	response := UsernameAvailabilityResponse{Available: isAvailable}
	if err != nil && !errors.Is(err, auth.ErrUsernameValidationFailed) {
		slog.Error("Username availability request failed", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_service_error", "")
		return
	}

	if errors.Is(err, auth.ErrUsernameValidationFailed) {
		response.ValidationError = err.Error()
	}

	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(response)

	if err != nil {
		slog.Error("UsernameAvailability: failed to encode response", "error", err)
	}
}

func statusForAuthError(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}
