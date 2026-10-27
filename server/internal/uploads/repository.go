package uploads

import (
	"context"
	"errors"
	"fmt"
	"io"
	"lensamity/internal/db"
	"lensamity/internal/storage"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type uploadRepository struct {
	store *db.Store
	s3    *storage.Client
}

func newUploadRepository(store *db.Store, s3 *storage.Client) (*uploadRepository, error) {
	if store == nil || store.Queries == nil || store.Pool == nil {
		return nil, errors.New("new photo repository: nil postgres store")
	}

	if s3 == nil || s3.Presign == nil || s3.Client == nil || s3.Bucket == "" {
		return nil, errors.New("new photo repository: invalid s3 client data")
	}

	return &uploadRepository{
		store: store,
		s3:    s3,
	}, nil
}

type presignPutObjectParams struct {
	Key         string
	ContentType string
	MaxSize     int64
	ExpiresAt   time.Time
}

func (r *uploadRepository) presignPostObject(ctx context.Context, p presignPutObjectParams) (*s3.PresignedPostRequest, error) {
	slog.Info("presignPostObject", "content_type", p.ContentType)
	req, err := r.s3.Presign.PresignPostObject(
		ctx,
		&s3.PutObjectInput{
			Bucket:      aws.String(r.s3.Bucket),
			Key:         aws.String(p.Key),
			ContentType: aws.String(p.ContentType),
		},
		func(options *s3.PresignPostOptions) {
			options.Expires = time.Until(p.ExpiresAt)
			options.Conditions = []any{
				[]any{"content-length-range", 1, p.MaxSize},
				map[string]string{"Content-Type": p.ContentType},
			}
		})
	if err != nil {
		return nil, fmt.Errorf("presign put object: %w", err)
	}
	return req, nil
}

type objectHeadData struct {
	ContentType string
	Size        int64
}

func (r *uploadRepository) headObject(ctx context.Context, bucket string, key string) (*objectHeadData, error) {
	head, err := r.s3.Client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if storage.IsObjectNotFound(err) {
			return nil, ErrUploadNotFound
		}
		return nil, fmt.Errorf("s3 head object: %w", err)
	}

	if head.ContentType == nil || head.ContentLength == nil || *head.ContentLength <= 0 {
		return nil, ErrUploadMetadataMissing
	}

	return &objectHeadData{ContentType: *head.ContentType, Size: *head.ContentLength}, nil
}

func (r *uploadRepository) getObject(ctx context.Context, bucket string, key string) (io.ReadCloser, error) {
	object, err := r.s3.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if storage.IsObjectNotFound(err) {
			return nil, ErrUploadNotFound
		}
		return nil, fmt.Errorf("s3 get object: %w", err)
	}

	return object.Body, nil
}

func (r *uploadRepository) deleteObject(ctx context.Context, bucket string, key string) error {
	_, err := r.s3.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if storage.IsObjectNotFound(err) {
			return nil
		}
		return fmt.Errorf("s3 delete object: %w", err)
	}
	return nil
}

type createPendingPhotoUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Key         string
	ContentType string
	Size        int64
	PhotoDate   time.Time
	Title       string
	Description string
	ExpiresAt   time.Time
}

func (r *uploadRepository) createPendingPhotoUpload(ctx context.Context, p createPendingPhotoUploadParams) error {
	queryParams := db.CreatePendingPhotoUploadRecordParams{
		ID:                p.ID,
		OwnerUserID:       p.OwnerUserID,
		Bucket:            r.s3.Bucket,
		ObjectKeyOriginal: p.Key,
		ContentType:       p.ContentType,
		Size:              p.Size,
		PhotoDate: pgtype.Date{
			Time:  p.PhotoDate,
			Valid: true,
		},
		ExpiresAt: p.ExpiresAt,
	}

	if strings.TrimSpace(p.Title) != "" {
		queryParams.Title = pgtype.Text{
			String: strings.TrimSpace(p.Title),
			Valid:  true,
		}
	}

	if strings.TrimSpace(p.Description) != "" {
		queryParams.Description = pgtype.Text{
			String: strings.TrimSpace(p.Description),
			Valid:  true,
		}
	}

	err := r.store.Queries.CreatePendingPhotoUploadRecord(ctx, queryParams)
	if err != nil {
		return fmt.Errorf("createPendingPhotoUpload: %w", err)
	}
	return nil
}

type createPendingAvatarUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Key         string
	ContentType string
	Size        int64
	ExpiresAt   time.Time
}

func (r *uploadRepository) createPendingAvatarUpload(ctx context.Context, p createPendingAvatarUploadParams) error {
	err := r.store.Queries.CreatePendingAvatarUpload(ctx, db.CreatePendingAvatarUploadParams{
		ID:          p.ID,
		UserID:      p.OwnerUserID,
		Bucket:      r.s3.Bucket,
		ObjectKey:   p.Key,
		ContentType: p.ContentType,
		Size:        p.Size,
		ExpiresAt:   p.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("create pending avatar upload: %w", err)
	}
	return nil
}

type pendingAvatarUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
}

type pendingAvatarUploadData struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Bucket      string
	Key         string
	ContentType string
	Size        int64
}

type avatarObjectData struct {
	OwnerUserID uuid.UUID
	Bucket      string
	Key         string
}

func (r *uploadRepository) pendingAvatarUpload(ctx context.Context, p pendingAvatarUploadParams) (*pendingAvatarUploadData, error) {
	row, err := r.store.Queries.MarkAvatarUploadProcessing(ctx, db.MarkAvatarUploadProcessingParams{
		ID:     p.ID,
		UserID: p.OwnerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("mark avatar upload processing: %w", err)
	}

	return &pendingAvatarUploadData{
		ID:          row.ID,
		OwnerUserID: row.UserID,
		Bucket:      row.Bucket,
		Key:         row.ObjectKey,
		ContentType: row.ContentType,
		Size:        row.Size,
	}, nil
}

func (r *uploadRepository) avatarUploadStatus(ctx context.Context, p pendingAvatarUploadParams) (string, error) {
	status, err := r.store.Queries.GetAvatarUploadStatus(ctx, db.GetAvatarUploadStatusParams{ID: p.ID, UserID: p.OwnerUserID})
	if err != nil {
		return "", fmt.Errorf("get avatar upload status: %w", err)
	}
	return status, nil
}

type failPendingAvatarUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Reason      string
}

func (r *uploadRepository) failPendingAvatarUpload(ctx context.Context, p failPendingAvatarUploadParams) error {
	err := r.store.Queries.MarkAvatarUploadFailed(ctx, db.MarkAvatarUploadFailedParams{
		ID:     p.ID,
		UserID: p.OwnerUserID,
		FailureReason: pgtype.Text{
			String: p.Reason,
			Valid:  true,
		},
	})
	if err != nil {
		return fmt.Errorf("mark avatar upload failed: %w", err)
	}
	return nil
}

func (r *uploadRepository) promoteAvatarUpload(ctx context.Context, p pendingAvatarUploadData) (*db.UserAvatar, error) {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin avatar promotion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	q := r.store.Queries.WithTx(tx)
	previous, err := q.GetCurrentAvatarForUpdate(ctx, p.OwnerUserID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lock current avatar: %w", err)
	}
	if err == nil {
		if err := q.MarkAvatarUploadSuperseded(ctx, db.MarkAvatarUploadSupersededParams{
			UserID:    p.OwnerUserID,
			Bucket:    previous.Bucket,
			ObjectKey: previous.ObjectKey,
		}); err != nil {
			return nil, fmt.Errorf("mark replaced avatar upload superseded: %w", err)
		}
	}

	if err := q.MarkAvatarUploadReady(ctx, db.MarkAvatarUploadReadyParams{ID: p.ID, UserID: p.OwnerUserID}); err != nil {
		return nil, fmt.Errorf("mark avatar upload ready: %w", err)
	}
	if err := q.UpsertCurrentAvatar(ctx, db.UpsertCurrentAvatarParams{
		UserID:      p.OwnerUserID,
		Bucket:      p.Bucket,
		ObjectKey:   p.Key,
		ContentType: p.ContentType,
		Size:        p.Size,
	}); err != nil {
		return nil, fmt.Errorf("set current avatar: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit avatar promotion: %w", err)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &previous, nil
}

func (r *uploadRepository) markAvatarUploadDeleted(ctx context.Context, p avatarObjectData) error {
	if err := r.store.Queries.MarkAvatarUploadDeleted(ctx, db.MarkAvatarUploadDeletedParams{
		UserID:    p.OwnerUserID,
		Bucket:    p.Bucket,
		ObjectKey: p.Key,
	}); err != nil {
		return fmt.Errorf("mark avatar upload deleted: %w", err)
	}
	return nil
}

type pendingPhotoUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
}

type pendingPhotoUploadData struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Bucket      string
	Key         string
	ContentType string
	Size        int64
	PhotoDate   time.Time
	Status      string
}

func (r *uploadRepository) pendingPhotoUpload(ctx context.Context, p pendingPhotoUploadParams) (*pendingPhotoUploadData, error) {
	row, err := r.store.Queries.MarkPhotoUploadRecordProcessing(ctx, db.MarkPhotoUploadRecordProcessingParams{
		ID:          p.ID,
		OwnerUserID: p.OwnerUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("pendingPhotoUpload: %w", err)
	}

	return &pendingPhotoUploadData{
		ID:          row.ID,
		OwnerUserID: row.OwnerUserID,
		Bucket:      row.Bucket,
		Key:         row.ObjectKeyOriginal,
		ContentType: row.ContentType,
		Size:        row.Size,
		PhotoDate:   row.PhotoDate.Time,
		Status:      string(row.Status),
	}, nil
}

type completePendingPhotoUploadParams struct {
	ID                 uuid.UUID
	OwnerUserID        uuid.UUID
	ObjectKeyProcessed []byte
	Width              int32
	Height             int32
}

func (r *uploadRepository) completePendingPhotoUpload(ctx context.Context, p completePendingPhotoUploadParams) error {
	queryParams := db.MarkProcessedPhotoUploadRecordCompletedParams{
		ID:                 p.ID,
		OwnerUserID:        p.OwnerUserID,
		ObjectKeyProcessed: p.ObjectKeyProcessed,
		Width: pgtype.Int4{
			Int32: p.Width,
			Valid: p.Width > 0 && p.Height > 0,
		},
		Height: pgtype.Int4{
			Int32: p.Height,
			Valid: p.Width > 0 && p.Height > 0,
		},
	}

	_, err := r.store.Queries.MarkProcessedPhotoUploadRecordCompleted(ctx, queryParams)
	if err != nil {
		return fmt.Errorf("completePendingPhotoUpload: %w", err)
	}
	return nil
}

type failPendingPhotoUploadParams struct {
	ID          uuid.UUID
	OwnerUserID uuid.UUID
	Reason      string
}

func (r *uploadRepository) failPendingPhotoUpload(ctx context.Context, p failPendingPhotoUploadParams) error {
	_, err := r.store.Queries.MarkProcessedPhotoUploadRecordFailed(ctx, db.MarkProcessedPhotoUploadRecordFailedParams{
		ID:          p.ID,
		OwnerUserID: p.OwnerUserID,
		FailureReason: pgtype.Text{
			String: p.Reason,
			Valid:  true,
		},
	})
	if err != nil {
		return fmt.Errorf("failPendingPhotoUpload: %w", err)
	}
	return nil
}

type deletePhotoRecordParams struct {
	PhotoID     uuid.UUID
	OwnerUserID uuid.UUID
}

func (r *uploadRepository) deletePhotoRecord(ctx context.Context, p deletePhotoRecordParams) error {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	q := r.store.Queries.WithTx(tx)

	row, err := q.MarkPhotoRecordDeleted(ctx, db.MarkPhotoRecordDeletedParams{PhotoID: p.PhotoID, OwnerUserID: p.OwnerUserID})
	if err != nil {
		return err
	}

	if err := r.deleteObject(ctx, row.Bucket, row.ObjectKeyOriginal); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (r *uploadRepository) deleteAvatar(ctx context.Context, userID uuid.UUID) (*avatarObjectData, error) {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin avatar deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	avatar, err := r.store.Queries.WithTx(tx).DeleteCurrentAvatar(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("delete current avatar record: %w", err)
	}
	if err := r.store.Queries.WithTx(tx).MarkAvatarUploadSuperseded(ctx, db.MarkAvatarUploadSupersededParams{
		UserID:    userID,
		Bucket:    avatar.Bucket,
		ObjectKey: avatar.ObjectKey,
	}); err != nil {
		return nil, fmt.Errorf("mark deleted avatar upload superseded: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit avatar deletion: %w", err)
	}
	return &avatarObjectData{OwnerUserID: userID, Bucket: avatar.Bucket, Key: avatar.ObjectKey}, nil
}
