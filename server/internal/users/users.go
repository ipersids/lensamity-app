package users

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"lensamity/internal/auth"
	"lensamity/internal/db"
	"lensamity/internal/storage"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type UserService struct {
	repo usersStore
}

type usersStore interface {
	presignURL(ctx context.Context, p presignURLparams) (*v4.PresignedHTTPRequest, error)
	profile(ctx context.Context, p profileParams) (*db.GetUserProfileRow, error)
	accessProfile(ctx context.Context, p profileParams) (*db.GetUserAccessProfileRow, error)
	photosPage(ctx context.Context, p photosPageParams) ([]db.Photo, error)
	updateProfile(ctx context.Context, p updateProfileParams) error
	updateUsername(ctx context.Context, userID uuid.UUID, newUsername string) (*db.UpdateUsernameRow, error)
	deleteProfile(ctx context.Context, userID uuid.UUID) error
	photoByID(ctx context.Context, p photoByIDParams) (*db.GetPhotoByIDRow, error)
}

func NewUserService(store *db.Store, s3Client *storage.Client) (*UserService, error) {
	if store == nil || s3Client == nil {
		return nil, errors.New("new users service: nil postgres or s3 data")
	}

	repo, err := newUsersRepository(store, s3Client)
	if err != nil {
		return nil, err
	}

	return &UserService{repo: repo}, nil
}

var (
	ErrorGetUserProfile        = errors.New("user profile not found")
	ErrorGetUserPhotoPage      = errors.New("photos not found")
	ErrorInvalidCursor         = errors.New("invalid cursor")
	ErrorInvalidAboutLength    = errors.New("about is longer than 300 characters")
	ErrorNewUsernameInvalid    = errors.New("invalid new username")
	ErrorNewUsernameTaken      = errors.New("new username is already taken")
	ErrorFailedToDeleteProfile = errors.New("failed to delete profile")
	ErrorForbiddenPhotoAccess  = errors.New("can't view this content")
	ErrorNotFoundPhotoID       = errors.New("photo not found")
)

const (
	usernameKeyUniqueConstraint = "users_username_key_key"
)

type GetUserProfileResult struct {
	ID                     uuid.UUID
	Username               string
	DisplayName            string
	About                  string
	PhotoCount             int64
	Visibility             string
	JoinedAt               time.Time
	AvatarPresignedRequest *v4.PresignedHTTPRequest
}

func (s *UserService) GetUserProfile(ctx context.Context, username string) (*GetUserProfileResult, error) {
	profile, err := s.repo.profile(ctx, profileParams{Username: username})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrorGetUserProfile, err)
		}
		return nil, fmt.Errorf("get user profile: %w", err)
	}

	result := &GetUserProfileResult{
		ID:          profile.ID,
		Username:    profile.UsernameKey,
		DisplayName: profile.UsernameDisplay,
		PhotoCount:  profile.PhotoCount,
		Visibility:  profile.ProfileVisibility,
		JoinedAt:    profile.JoinedAt.Time,
	}

	if profile.About.Valid {
		result.About = profile.About.String
	}

	if !profile.AvatarObjectKey.Valid || !profile.AvatarBucket.Valid {
		return result, nil
	}

	avatarReq, err := s.repo.presignURL(ctx, presignURLparams{
		Bucket:    profile.AvatarBucket.String,
		Key:       profile.AvatarObjectKey.String,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		if storage.IsObjectNotFound(err) {
			slog.Error("user profile avatar object not found", "error", err)
			return result, nil
		}
		return nil, fmt.Errorf("get user profile avatar url: %w", err)
	}

	result.AvatarPresignedRequest = avatarReq

	return result, nil
}

type GetUserPhotosParams struct {
	OwnerUsername string
	ViewerID      uuid.UUID
	Limit         int32
	Cursor        string
}

type GetUserPhotosResult struct {
	Items         []Photo
	NextCursor    string
	CanEdit       bool
	CanViewPhotos bool
}

type PhotoRequest struct {
	URL     string      `json:"url"`
	Method  string      `json:"method"`
	Header  http.Header `json:"header"`
	IsReady bool        `json:"isReady"`
}

type Photo struct {
	ID          uuid.UUID    `json:"photoID"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Date        string       `json:"date"`
	Request     PhotoRequest `json:"request"`
}

func (s *UserService) GetUserPhotos(ctx context.Context, p GetUserPhotosParams) (*GetUserPhotosResult, error) {
	accessProfile, err := s.repo.accessProfile(ctx, profileParams{Username: p.OwnerUsername})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrorGetUserProfile, err)
		}
		return nil, fmt.Errorf("get user profile: %w", err)
	}

	result := GetUserPhotosResult{
		CanEdit:       p.ViewerID == accessProfile.ID,
		CanViewPhotos: canViewPhotos(owner{ID: accessProfile.ID, Visibility: accessProfile.ProfileVisibility}, p.ViewerID),
	}

	if !result.CanViewPhotos {
		return &result, nil
	}

	photoPageParams := photosPageParams{
		OwnerID:  accessProfile.ID,
		Limit:    p.Limit + 1,
		CursorID: uuid.Nil,
	}

	if p.Cursor != "" {
		cursorDate, cursorID, err := decodePhotoCursor(p.Cursor)
		if err != nil {
			return nil, err
		}
		photoPageParams.CursorDate = cursorDate
		photoPageParams.CursorID = cursorID
	}

	photos, err := s.repo.photosPage(ctx, photoPageParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrorGetUserProfile, err)
		}
		return nil, fmt.Errorf("get user photos: %w", err)
	}

	if len(photos) > int(p.Limit) {
		photos = photos[:p.Limit]
		last := photos[len(photos)-1]
		nextCursor, err := encodePhotoCursor(last.PhotoDate.Time, last.ID)
		if err != nil {
			return nil, err
		}
		result.NextCursor = nextCursor
	}

	result.Items = make([]Photo, 0, len(photos))

	for _, photo := range photos {
		item := Photo{
			ID:          photo.ID,
			Title:       photo.Title.String,
			Description: photo.Description.String,
			Date:        photo.PhotoDate.Time.Format("2006-01-02"),
			Request: PhotoRequest{
				IsReady: false,
			},
		}

		photoReq, err := s.repo.presignURL(ctx, presignURLparams{
			Bucket:    photo.Bucket,
			Key:       photo.ObjectKeyOriginal,
			ExpiresAt: time.Now().Add(time.Hour),
		})
		if err != nil {
			if storage.IsObjectNotFound(err) {
				slog.Error("photo object object not found", "error", err)
				result.Items = append(result.Items, item)
				continue
			}
			return nil, fmt.Errorf("get photo object url: %w", err)
		}

		item.Request = PhotoRequest{
			IsReady: true,
			URL:     photoReq.URL,
			Method:  photoReq.Method,
			Header:  photoReq.SignedHeader,
		}
		result.Items = append(result.Items, item)
	}

	return &result, nil
}

type GetUserPhotoByIDParams struct {
	OwnerUsername string
	ViewerID      uuid.UUID
	PhotoID       uuid.UUID
}

type GetUserPhotoByIDResult struct {
	Photo           Photo
	PreviousPhotoId uuid.UUID
	NextPhotoID     uuid.UUID
}

func (s *UserService) GetUserPhotoByID(ctx context.Context, p GetUserPhotoByIDParams) (*GetUserPhotoByIDResult, error) {
	accessProfile, err := s.repo.accessProfile(ctx, profileParams{Username: p.OwnerUsername})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrorGetUserProfile, err)
		}
		return nil, fmt.Errorf("get photo by id: %w", err)
	}

	if !canViewPhotos(owner{ID: accessProfile.ID, Visibility: accessProfile.ProfileVisibility}, p.ViewerID) {
		return nil, ErrorForbiddenPhotoAccess
	}

	photoData, err := s.repo.photoByID(ctx, photoByIDParams{ownerID: accessProfile.ID, photoID: p.PhotoID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %w", ErrorNotFoundPhotoID, err)
		}
		return nil, fmt.Errorf("get photo by id: %w", err)
	}

	photoReq, err := s.repo.presignURL(ctx, presignURLparams{
		Bucket:    photoData.Bucket,
		Key:       photoData.ObjectKeyOriginal,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		return nil, err
	}

	return &GetUserPhotoByIDResult{
		Photo: Photo{
			ID:          photoData.ID,
			Title:       photoData.Title.String,
			Description: photoData.Description.String,
			Date:        photoData.PhotoDate.Time.Format("2006-01-02"),
			Request: PhotoRequest{
				URL:     photoReq.URL,
				Method:  photoReq.Method,
				Header:  photoReq.SignedHeader,
				IsReady: true,
			},
		},
		PreviousPhotoId: photoData.PreviousPhotoID,
		NextPhotoID:     photoData.NextPhotoID,
	}, nil
}

type UpdateProfileParams struct {
	OwnerID     uuid.UUID
	DisplayName string
	About       string
	Visibility  string
}

func (s *UserService) UpdateProfile(ctx context.Context, p UpdateProfileParams) error {
	displayNameNormalized := auth.NormText(p.DisplayName)
	err := auth.ValidateNameLength(displayNameNormalized)
	if err != nil {
		return err
	}

	aboutNormalized := auth.NormText(p.About)
	err = ValidateAboutLength(aboutNormalized)
	if err != nil {
		return err
	}

	return s.repo.updateProfile(ctx, updateProfileParams{
		OwnerID:     p.OwnerID,
		DisplayName: displayNameNormalized,
		About:       aboutNormalized,
		Visibility:  p.Visibility,
	})
}

type UpdateUsernameParams struct {
	UserID      uuid.UUID
	Username    string
	NewUsername string
}

func (s *UserService) UpdateUsername(ctx context.Context, p UpdateUsernameParams) (usename string, err error) {
	newUsernameNormalized := auth.NormKey(p.NewUsername)
	if err := auth.ValidateUsernameKey(newUsernameNormalized); err != nil {
		return "", fmt.Errorf("%w: %w", ErrorNewUsernameInvalid, err)
	}

	if newUsernameNormalized == p.Username {
		return p.Username, nil
	}

	row, err := s.repo.updateUsername(ctx, p.UserID, newUsernameNormalized)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == usernameKeyUniqueConstraint {
			return "", ErrorNewUsernameTaken
		}
		return "", err
	}

	return row.UsernameKey, nil
}

func (s *UserService) DeleteProfile(ctx context.Context, userID uuid.UUID) error {
	if err := s.repo.deleteProfile(ctx, userID); err != nil {
		return fmt.Errorf("%w: %w", ErrorFailedToDeleteProfile, err)
	}

	return nil
}

type owner struct {
	ID         uuid.UUID
	Visibility string
}

func canViewPhotos(owner owner, viewerID uuid.UUID) bool {
	return owner.Visibility == "public" || owner.ID == viewerID
}

type photoCursor struct {
	PhotoDate string    `json:"photoDate"`
	ID        uuid.UUID `json:"id"`
}

func encodePhotoCursor(photoDate time.Time, id uuid.UUID) (string, error) {
	payload := photoCursor{
		PhotoDate: photoDate.Format(time.DateOnly),
		ID:        id,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode photo page cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodePhotoCursor(cursor string) (time.Time, uuid.UUID, error) {
	if cursor == "" {
		return time.Time{}, uuid.Nil, ErrorInvalidCursor
	}

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: %w", ErrorInvalidCursor, err)
	}

	var payload photoCursor
	if err := json.Unmarshal(raw, &payload); err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w json: %w", ErrorInvalidCursor, err)
	}

	photoDate, err := time.Parse(time.DateOnly, payload.PhotoDate)
	if err != nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w date: %w", ErrorInvalidCursor, err)
	}

	if payload.ID == uuid.Nil {
		return time.Time{}, uuid.Nil, fmt.Errorf("%w: empty id", ErrorInvalidCursor)
	}

	return photoDate, payload.ID, nil
}

func ValidateAboutLength(name string) error {
	l := utf8.RuneCountInString(name)

	if l > 300 {
		return ErrorInvalidAboutLength
	}

	return nil
}
