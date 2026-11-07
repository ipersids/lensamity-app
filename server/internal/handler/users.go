package handler

import (
	"context"
	"encoding/json"
	"errors"
	"lensamity/internal/auth"
	"lensamity/internal/middleware"
	"lensamity/internal/users"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type UserHandler struct {
	userService *users.UserService
}

func NewUserHandler(userService *users.UserService) (*UserHandler, error) {
	if userService == nil {
		return nil, errors.New("handler: nil user service")
	}

	return &UserHandler{
		userService: userService,
	}, nil
}

type AvatarResponse struct {
	URL    string      `json:"url"`
	Method string      `json:"method"`
	Header http.Header `json:"header"`
}

type GetUserProfileResponse struct {
	Username      string          `json:"username"`
	DisplayName   string          `json:"displayName"`
	PhotoCount    int64           `json:"photoCount"`
	CanEdit       bool            `json:"canEdit"`
	CanViewPhotos bool            `json:"canViewPhotos"`
	Visibility    string          `json:"visibility"`
	JoinedAt      time.Time       `json:"joinedAt"`
	Avatar        *AvatarResponse `json:"avatar,omitempty"`
	About         string          `json:"about,omitempty"`
}

func (h *UserHandler) GetUserProfile(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	user, err := h.userService.GetUserProfile(ctx, username)
	if err != nil {
		if errors.Is(err, users.ErrorGetUserProfile) {
			slog.Error("UserProfile not found", "error", err)
			http.NotFound(w, r)
			return
		}
		slog.Error("UserProfile: request failed", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	profile := GetUserProfileResponse{
		Username:      user.Username,
		DisplayName:   user.DisplayName,
		About:         user.About,
		PhotoCount:    user.PhotoCount,
		CanEdit:       user.ID == userID,
		CanViewPhotos: user.Visibility == "public" || user.ID == userID,
		Visibility:    user.Visibility,
		JoinedAt:      user.JoinedAt,
	}

	if user.AvatarPresignedRequest != nil {
		profile.Avatar = &AvatarResponse{
			Method: user.AvatarPresignedRequest.Method,
			URL:    user.AvatarPresignedRequest.URL,
			Header: user.AvatarPresignedRequest.SignedHeader,
		}
	}

	err = json.NewEncoder(w).Encode(profile)

	if err != nil {
		slog.Error("UserProfile handler: failed to encode response", "error", err)
	}
}

type GetUserPhotosResponse struct {
	CanEdit       bool          `json:"canEdit"`
	CanViewPhotos bool          `json:"canViewPhotos"`
	PhotoCount    int           `json:"photoCount"`
	Items         []users.Photo `json:"items"`
	NextCursor    string        `json:"nextCursor,omitempty"`
}

func (h *UserHandler) GetUserPhotos(w http.ResponseWriter, r *http.Request) {
	ownerUsername := r.PathValue("username")

	query := r.URL.Query()

	limit := int32(24)
	if rawLimit := query.Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.ParseInt(rawLimit, 10, 32)
		if err != nil || parsedLimit < 1 {
			WriteError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive number")
			return
		}
		if parsedLimit > 100 {
			parsedLimit = 100
		}
		limit = int32(parsedLimit)
	}

	cursor := query.Get("cursor")

	viewerID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	photos, err := h.userService.GetUserPhotos(ctx, users.GetUserPhotosParams{
		ViewerID:      viewerID,
		OwnerUsername: ownerUsername,
		Limit:         limit,
		Cursor:        cursor,
	})
	if err != nil {
		if errors.Is(err, users.ErrorGetUserProfile) {
			slog.Error("GetUserPhotos not found", "error", err)
			http.NotFound(w, r)
			return
		}
		slog.Error("GetUserPhotos: request failed", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	response := GetUserPhotosResponse{
		PhotoCount:    len(photos.Items),
		Items:         photos.Items,
		CanEdit:       photos.CanEdit,
		CanViewPhotos: photos.CanViewPhotos,
		NextCursor:    photos.NextCursor,
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		slog.Error("GetUserPhotos handler: failed to encode response", "error", err)
	}
}

type GetUserPhotoByIDResponse struct {
	Photo           users.Photo `json:"photo"`
	PreviousPhotoID *uuid.UUID  `json:"previousPhotoID,omitempty"`
	NextPhotoID     *uuid.UUID  `json:"nextPhotoID,omitempty"`
}

func (h *UserHandler) GetUserPhotoByID(w http.ResponseWriter, r *http.Request) {
	ownerUsername := r.PathValue("username")

	id := r.PathValue("id")
	photoID, err := uuid.Parse(id)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_id", "malformed ID")
		return
	}

	viewerID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx := r.Context()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := h.userService.GetUserPhotoByID(ctx, users.GetUserPhotoByIDParams{
		OwnerUsername: ownerUsername,
		ViewerID:      viewerID,
		PhotoID:       photoID,
	})
	if err != nil {
		slog.Error("GetUserPhotoByID: request failed", "error", err)
		if errors.Is(err, users.ErrorForbiddenPhotoAccess) {
			http.Error(w, "photo is private or unavailable to view", http.StatusForbidden)
			return
		}
		if errors.Is(err, users.ErrorNotFoundPhotoID) {
			http.Error(w, "photo not found", http.StatusNotFound)
			return
		}
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	body := GetUserPhotoByIDResponse{
		Photo:           res.Photo,
		PreviousPhotoID: nil,
		NextPhotoID:     nil,
	}

	if res.NextPhotoID != uuid.Nil {
		body.NextPhotoID = &res.NextPhotoID
	}

	if res.PreviousPhotoId != uuid.Nil {
		body.PreviousPhotoID = &res.PreviousPhotoId
	}

	err = json.NewEncoder(w).Encode(body)
	if err != nil {
		slog.Error("GetUserPhotoByID handler: failed to encode response", "error", err)
	}

}

type UpdateMyProfileRequest struct {
	DisplayName string `json:"displayName"`
	About       string `json:"about"`
	Visibility  string `json:"visibility"`
}

func (h *UserHandler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

	var req UpdateMyProfileRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON")
		return
	}

	if req.DisplayName == "" {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON: display name should not be empty")
		return
	}

	if req.Visibility == "" || !(req.Visibility == "public" || req.Visibility == "private") {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON: unexpected visibility value")
		return
	}

	ownerID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := h.userService.UpdateProfile(ctx, users.UpdateProfileParams{
		OwnerID:     ownerID,
		DisplayName: req.DisplayName,
		About:       req.About,
		Visibility:  req.Visibility,
	})
	if err != nil {
		slog.Error("UpdateMyProfile: request failed", "error", err)
		if errors.Is(err, auth.ErrDisplayNameLength) || errors.Is(err, users.ErrorInvalidAboutLength) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type UpdateMyUsernameRequest struct {
	NewUsername string `json:"newUsername"`
}

type UpdateMyUsernameResponse struct {
	UpdatedUsername string `json:"updatedUsername"`
}

func (h *UserHandler) UpdateMyUsername(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)

	var req UpdateMyUsernameRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed_json", "malformed JSON")
		return
	}

	if strings.TrimSpace(req.NewUsername) == "" {
		WriteError(w, http.StatusBadRequest, "malformed_json", "username is empty")
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	updatedUsername, err := h.userService.UpdateUsername(ctx, users.UpdateUsernameParams{
		UserID:      userID,
		Username:    username,
		NewUsername: req.NewUsername,
	})
	if err != nil {
		slog.Error("UpdateUsername: request failed", "error", err)
		if errors.Is(err, users.ErrorNewUsernameInvalid) {
			http.Error(w, "username contains forbidden characters", http.StatusBadRequest)
			return
		}
		if errors.Is(err, users.ErrorNewUsernameTaken) {
			http.Error(w, "username is not available", http.StatusBadRequest)
			return
		}
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(UpdateMyUsernameResponse{UpdatedUsername: updatedUsername})
	if err != nil {
		slog.Error("UpdateUsername handler: failed to encode response", "error", err)
	}
}

func (h *UserHandler) DeleteMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := h.userService.DeleteProfile(ctx, userID)
	if err != nil {
		slog.Error("DeleteMyProfile request failed", "error", err)
		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	middleware.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
