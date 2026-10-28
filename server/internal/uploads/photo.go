package uploads

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"lensamity/internal/db"
	"lensamity/internal/storage"
	"log/slog"
	"mime"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	_ "golang.org/x/image/webp"
)

var (
	ErrInternal              = errors.New("internal error")
	ErrUnsupportedFileType   = errors.New("unsupported file type")
	ErrFileTooLarge          = errors.New("file size exceeded limits")
	ErrInvalidImage          = errors.New("invalid image")
	ErrImageDimensions       = errors.New("image dimensions exceeded limits")
	ErrImageAspectRatio      = errors.New("image aspect ratio exceeded limits")
	ErrImageTooManyPixels    = errors.New("image pixel count exceeded limits")
	ErrPhotoNotFound         = errors.New("photo not found")
	ErrAvatarUploadNotFound  = errors.New("avatar upload not found or expired")
	ErrPhotoNotCompletable   = errors.New("photo cannot be completed")
	ErrUploadNotFound        = errors.New("uploaded object not found")
	ErrUploadMetadataMissing = errors.New("uploaded object is missing content type or length")
	ErrUploadSizeMismatch    = errors.New("uploaded object size mismatch")
	ErrPhotoAlreadyExists    = errors.New("photo already exists for date")
	ErrDateOutOfRange        = errors.New("date must be within the last 7 days including today")
	ErrUnauthorized          = errors.New("unauthorized")
)

const maxImageBytes int64 = 10 * 1024 * 1024
const maxAvatarBytes int64 = 1 * 1024 * 1024
const maxAvatarDimension = 4_096
const maxAvatarPixels int64 = 16_000_000
const maxAvatarAspectRatio = 3

type PhotoService struct {
	repo photoStore
	now  func() time.Time
}

type photoStore interface {
	presignPostObject(ctx context.Context, p presignPutObjectParams) (*s3.PresignedPostRequest, error)
	createPendingPhotoUpload(ctx context.Context, p createPendingPhotoUploadParams) error
	pendingPhotoUpload(ctx context.Context, p pendingPhotoUploadParams) (*pendingPhotoUploadData, error)
	completePendingPhotoUpload(ctx context.Context, p completePendingPhotoUploadParams) error
	failPendingPhotoUpload(ctx context.Context, p failPendingPhotoUploadParams) error
	headObject(ctx context.Context, bucket string, key string) (*objectHeadData, error)
	getObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error)
	deletePhotoRecord(ctx context.Context, p deletePhotoRecordParams) error
	deleteAvatar(ctx context.Context, userID uuid.UUID) error
	createPendingAvatarUpload(ctx context.Context, p createPendingAvatarUploadParams) error
	pendingAvatarUpload(ctx context.Context, p pendingAvatarUploadParams) (*pendingAvatarUploadData, error)
	avatarUploadStatus(ctx context.Context, p pendingAvatarUploadParams) (string, error)
	failPendingAvatarUpload(ctx context.Context, p failPendingAvatarUploadParams) error
	promoteAvatarUpload(ctx context.Context, p pendingAvatarUploadData) error
}

func NewPhotoService(store *db.Store, s3Client *storage.Client) (*PhotoService, error) {
	if store == nil || s3Client == nil {
		return nil, errors.New("new photo service: nil postgres or s3 data")
	}

	repo, err := newUploadRepository(store, s3Client)
	if err != nil {
		return nil, err
	}

	return &PhotoService{
		repo: repo,
		now:  time.Now,
	}, nil
}

type UploadPhotoIntentParams struct {
	OwnerUserID uuid.UUID
	PhotoDate   time.Time
	ContentType string
	Size        int64
	Title       string
	Description string
}

type UploadPhotoIntentResult struct {
	PresignedRequest *s3.PresignedPostRequest
	PhotoID          uuid.UUID
}

func (ps *PhotoService) UploadPhotoIntent(ctx context.Context, p UploadPhotoIntentParams) (*UploadPhotoIntentResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", ErrInternal)
	}

	validated, err := ps.validateUploadPhotoIntentParams(&p)
	if err != nil {
		return nil, err
	}

	photoID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("%w: generate photo upload uuid: %w", ErrInternal, err)
	}

	key := fmt.Sprintf(
		"%s/photos/%s/%s/original.%s",
		validated.OwnerUserID.String(),
		validated.PhotoDate.Format("2006-01-02"),
		photoID.String(),
		validated.Extension,
	)
	expiresAt := ps.now().UTC().Add(10 * time.Minute)

	req, err := ps.repo.presignPostObject(ctx, presignPutObjectParams{
		Key:         key,
		ContentType: validated.ContentType,
		MaxSize:     maxImageBytes,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create presigned photo upload: %w", ErrInternal, err)
	}

	err = ps.repo.createPendingPhotoUpload(ctx, createPendingPhotoUploadParams{
		ID:          photoID,
		OwnerUserID: validated.OwnerUserID,
		Key:         key,
		ContentType: validated.ContentType,
		Size:        validated.Size,
		PhotoDate:   validated.PhotoDate,
		Title:       validated.Title,
		Description: validated.Description,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create pending photo upload: %w", ErrInternal, err)
	}

	return &UploadPhotoIntentResult{
		PresignedRequest: req,
		PhotoID:          photoID,
	}, nil
}

type UploadPhotoCompleteParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
}

func (ps *PhotoService) UploadPhotoComplete(ctx context.Context, p UploadPhotoCompleteParams) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInternal)
	}

	pendingUploadData, err := ps.repo.pendingPhotoUpload(ctx, pendingPhotoUploadParams{ID: p.ID, OwnerUserID: p.OwnerUserID})
	if err != nil {
		return fmt.Errorf("%w: complete photo upload: %w", ErrInternal, err)
	}

	if pendingUploadData.Status != string(db.UploadStatusProcessing) {
		return fmt.Errorf("%w: complete photo upload: unexpected status %s", ErrInternal, pendingUploadData.Status)
	}

	fail := func(reason string, cause error) error {
		if err := ps.repo.failPendingPhotoUpload(ctx, failPendingPhotoUploadParams{
			ID:          pendingUploadData.ID,
			OwnerUserID: pendingUploadData.OwnerUserID,
			Reason:      reason,
		}); err != nil {
			return errors.Join(cause, fmt.Errorf("%w: mark photo upload failed: %w", ErrInternal, err))
		}

		return cause
	}

	headData, err := ps.repo.headObject(ctx, pendingUploadData.Bucket, pendingUploadData.Key)
	if err != nil {
		switch {
		case errors.Is(err, ErrUploadNotFound):
			return fail("upload_not_found", err)
		case errors.Is(err, ErrUploadMetadataMissing):
			return fail("missing_object_metadata", err)
		default:
			return fail("head_object_failed", err)
		}
	}

	if headData.Size != pendingUploadData.Size {
		return fail("size_mismatch", ErrUploadSizeMismatch)
	}
	if headData.Size > maxImageBytes {
		return fail("file_too_large", ErrFileTooLarge)
	}

	// @TODO: validate real bytes, check actual image type, dimension, and encode a processed webp

	contentType, _, err := normalizeImageContentType(headData.ContentType)
	if err != nil {
		return fail("unsupported_content_type", err)
	}
	if contentType != pendingUploadData.ContentType {
		return fail("content_type_mismatch", ErrUnsupportedFileType)
	}

	if err := ps.repo.completePendingPhotoUpload(ctx, completePendingPhotoUploadParams{
		ID:                 p.ID,
		OwnerUserID:        p.OwnerUserID,
		ObjectKeyProcessed: []byte("{}"),
	}); err != nil {
		return fmt.Errorf("%w: complete photo upload: mark ready: %w", ErrInternal, err)
	}

	return nil
}

type DeletePhotoParams struct {
	OwnerID uuid.UUID
	PhotoID uuid.UUID
}

func (ps *PhotoService) DeletePhoto(ctx context.Context, p DeletePhotoParams) error {
	if err := ps.repo.deletePhotoRecord(ctx, deletePhotoRecordParams{PhotoID: p.PhotoID, OwnerUserID: p.OwnerID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPhotoNotFound
		}
		return err
	}

	return nil
}

func (ps *PhotoService) DeleteAvatar(ctx context.Context, userID uuid.UUID) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInternal)
	}
	if userID == uuid.Nil {
		return fmt.Errorf("%w: owner user id is required", ErrInternal)
	}

	if err := ps.repo.deleteAvatar(ctx, userID); err != nil {
		return fmt.Errorf("%w: delete avatar: %w", ErrInternal, err)
	}
	return nil
}

type UploadAvatarIntentParams struct {
	OwnerUserID uuid.UUID
	ContentType string
	Size        int64
}

type UploadAvatarIntentResult struct {
	PresignedRequest *s3.PresignedPostRequest
	UploadID         uuid.UUID
}

func (ps *PhotoService) UploadAvatarIntent(ctx context.Context, p UploadAvatarIntentParams) (*UploadAvatarIntentResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", ErrInternal)
	}

	if p.OwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: owner user id is required", ErrInternal)
	}

	contentType, ext, err := normalizeImageContentType(p.ContentType)
	if err != nil {
		return nil, err
	}

	if p.Size <= 0 || p.Size > maxAvatarBytes {
		return nil, ErrFileTooLarge
	}

	uploadID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("%w: generate avatar upload uuid: %w", ErrInternal, err)
	}

	key := fmt.Sprintf(
		"%s/avatars/%s/original.%s",
		p.OwnerUserID.String(),
		uploadID.String(),
		ext,
	)
	expiresAt := ps.now().UTC().Add(10 * time.Minute)

	req, err := ps.repo.presignPostObject(ctx, presignPutObjectParams{
		Key:         key,
		ContentType: contentType,
		MaxSize:     maxAvatarBytes,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create presigned avatar upload: %w", ErrInternal, err)
	}

	err = ps.repo.createPendingAvatarUpload(ctx, createPendingAvatarUploadParams{
		ID:          uploadID,
		OwnerUserID: p.OwnerUserID,
		Key:         key,
		ContentType: contentType,
		Size:        p.Size,
		ExpiresAt:   expiresAt,
	})

	if err != nil {
		return nil, fmt.Errorf("%w: create pending avatar upload: %w", ErrInternal, err)
	}

	return &UploadAvatarIntentResult{PresignedRequest: req, UploadID: uploadID}, nil
}

type UploadAvatarCompleteParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
}

func validateAvatarImage(r io.Reader, expectedContentType string) error {
	config, format, err := image.DecodeConfig(r)
	if err != nil {
		return fmt.Errorf("%w: decode image configuration", ErrInvalidImage)
	}

	contentType, ok := map[string]string{
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"webp": "image/webp",
	}[format]
	if !ok || contentType != expectedContentType {
		return ErrUnsupportedFileType
	}

	return validateAvatarDimensions(config.Width, config.Height)
}

func validateAvatarDimensions(width int, height int) error {
	if width <= 0 || height <= 0 {
		return ErrInvalidImage
	}
	if width > maxAvatarDimension || height > maxAvatarDimension {
		return ErrImageDimensions
	}

	pixels := int64(width) * int64(height)
	if pixels > maxAvatarPixels {
		return ErrImageTooManyPixels
	}

	if width > height*maxAvatarAspectRatio || height > width*maxAvatarAspectRatio {
		return ErrImageAspectRatio
	}

	return nil
}

func avatarImageFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrInvalidImage):
		return "invalid_image"
	case errors.Is(err, ErrImageDimensions):
		return "dimensions_exceeded"
	case errors.Is(err, ErrImageTooManyPixels):
		return "pixel_count_exceeded"
	case errors.Is(err, ErrImageAspectRatio):
		return "aspect_ratio_exceeded"
	default:
		return "unsupported_image"
	}
}

func (ps *PhotoService) UploadAvatarComplete(ctx context.Context, p UploadAvatarCompleteParams) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInternal)
	}
	if p.ID == uuid.Nil || p.OwnerUserID == uuid.Nil {
		return fmt.Errorf("%w: avatar upload id and owner user id are required", ErrInternal)
	}

	pendingUpload, err := ps.repo.pendingAvatarUpload(ctx, pendingAvatarUploadParams{ID: p.ID, OwnerUserID: p.OwnerUserID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			status, statusErr := ps.repo.avatarUploadStatus(ctx, pendingAvatarUploadParams{ID: p.ID, OwnerUserID: p.OwnerUserID})
			if statusErr == nil && status == "ready" {
				return nil
			}
			return ErrAvatarUploadNotFound
		}
		return fmt.Errorf("%w: prepare avatar upload completion: %w", ErrInternal, err)
	}

	fail := func(reason string, cause error) error {
		if err := ps.repo.failPendingAvatarUpload(ctx, failPendingAvatarUploadParams{
			ID:          pendingUpload.ID,
			OwnerUserID: pendingUpload.OwnerUserID,
			Reason:      reason,
		}); err != nil {
			return errors.Join(cause, fmt.Errorf("%w: mark avatar upload failed: %w", ErrInternal, err))
		}
		return cause
	}

	headData, err := ps.repo.headObject(ctx, pendingUpload.Bucket, pendingUpload.Key)
	if err != nil {
		if errors.Is(err, ErrUploadNotFound) {
			return fail("upload_not_found", err)
		}
		if errors.Is(err, ErrUploadMetadataMissing) {
			return fail("missing_object_metadata", err)
		}
		return fail("head_object_failed", err)
	}
	if headData.Size != pendingUpload.Size {
		return fail("size_mismatch", ErrUploadSizeMismatch)
	}
	if headData.Size > maxAvatarBytes {
		return fail("file_too_large", ErrFileTooLarge)
	}
	contentType, _, err := normalizeImageContentType(headData.ContentType)
	slog.Info("head object", "content_type", headData.ContentType, "size", headData.Size)
	if err != nil {
		return fail("unsupported_content_type", err)
	}
	if contentType != pendingUpload.ContentType {
		return fail("content_type_mismatch", ErrUnsupportedFileType)
	}

	object, err := ps.repo.getObject(ctx, pendingUpload.Bucket, pendingUpload.Key)
	if err != nil {
		if errors.Is(err, ErrUploadNotFound) {
			return fail("upload_not_found", err)
		}
		return fail("get_object_failed", err)
	}

	validationErr := validateAvatarImage(io.LimitReader(object, maxAvatarBytes+1), contentType)
	if closeErr := object.Close(); closeErr != nil {
		return fail("close_object_failed", fmt.Errorf("close avatar upload object: %w", closeErr))
	}
	if validationErr != nil {
		return fail(avatarImageFailureReason(validationErr), validationErr)
	}

	if err := ps.repo.promoteAvatarUpload(ctx, *pendingUpload); err != nil {
		return fmt.Errorf("%w: promote avatar upload: %w", ErrInternal, err)
	}

	return nil
}

type validatedUploadPhotoIntentParams struct {
	OwnerUserID uuid.UUID
	PhotoDate   time.Time
	ContentType string
	Extension   string
	Size        int64
	Title       string
	Description string
}

func (ps *PhotoService) validateUploadPhotoIntentParams(p *UploadPhotoIntentParams) (validatedUploadPhotoIntentParams, error) {
	if ps == nil {
		return validatedUploadPhotoIntentParams{}, fmt.Errorf("%w: nil photo service", ErrInternal)
	}
	if p == nil {
		return validatedUploadPhotoIntentParams{}, fmt.Errorf("%w: nil upload photo intent params", ErrInternal)
	}
	if p.OwnerUserID == uuid.Nil {
		return validatedUploadPhotoIntentParams{}, fmt.Errorf("%w: owner user id is required", ErrInternal)
	}

	photoDate := dateOnly(p.PhotoDate, time.UTC)
	if !withinLastSevenDays(photoDate, ps.now().UTC()) {
		return validatedUploadPhotoIntentParams{}, ErrDateOutOfRange
	}

	contentType, ext, err := normalizeImageContentType(p.ContentType)
	if err != nil {
		return validatedUploadPhotoIntentParams{}, err
	}

	if p.Size <= 0 || p.Size > maxImageBytes {
		return validatedUploadPhotoIntentParams{}, ErrFileTooLarge
	}

	return validatedUploadPhotoIntentParams{
		OwnerUserID: p.OwnerUserID,
		PhotoDate:   photoDate,
		ContentType: contentType,
		Extension:   ext,
		Size:        p.Size,
		Title:       p.Title,
		Description: p.Description,
	}, nil
}

func normalizeImageContentType(contentType string) (string, string, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", "", ErrUnsupportedFileType
	}

	switch mediaType {
	case "image/jpeg":
		return mediaType, "jpg", nil
	case "image/png":
		return mediaType, "png", nil
	case "image/webp":
		return mediaType, "webp", nil
	default:
		return "", "", ErrUnsupportedFileType
	}
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func withinLastSevenDays(date time.Time, now time.Time) bool {
	today := dateOnly(now, date.Location())
	minDate := today.AddDate(0, 0, -6)
	return !date.Before(minDate) && !date.After(today)
}
