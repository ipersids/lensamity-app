package users

import (
	"context"
	"errors"
	"fmt"
	"lensamity/internal/db"
	"lensamity/internal/storage"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type usersRepository struct {
	store *db.Store
	s3    *storage.Client
}

func newUsersRepository(store *db.Store, s3 *storage.Client) (*usersRepository, error) {
	if store == nil || store.Queries == nil || store.Pool == nil {
		return nil, errors.New("new users repository: nil postgres store")
	}

	if s3 == nil || s3.Presign == nil || s3.Client == nil || s3.Bucket == "" {
		return nil, errors.New("new users repository: invalid s3 client data")
	}

	return &usersRepository{
		store: store,
		s3:    s3,
	}, nil
}

type presignURLparams struct {
	Bucket    string
	Key       string
	ExpiresAt time.Time
}

func (r *usersRepository) presignURL(ctx context.Context, p presignURLparams) (*v4.PresignedHTTPRequest, error) {
	req, err := r.s3.Presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:          aws.String(p.Bucket),
		Key:             aws.String(p.Key),
		ResponseExpires: aws.Time(p.ExpiresAt),
	})
	if err != nil {
		return nil, err
	}

	return req, nil
}

type profileParams struct {
	Username string
}

func (r *usersRepository) profile(ctx context.Context, p profileParams) (*db.GetUserProfileRow, error) {
	row, err := r.store.Queries.GetUserProfile(ctx, p.Username)
	if err != nil {
		return nil, err
	}

	return &row, nil
}

func (r *usersRepository) accessProfile(ctx context.Context, p profileParams) (*db.GetUserAccessProfileRow, error) {
	row, err := r.store.Queries.GetUserAccessProfile(ctx, p.Username)
	if err != nil {
		return nil, err
	}

	return &row, nil
}

type photosPageParams struct {
	OwnerID    uuid.UUID
	CursorDate time.Time
	CursorID   uuid.UUID
	Limit      int32
}

func (r *usersRepository) photosPage(ctx context.Context, p photosPageParams) ([]db.Photo, error) {
	var rows []db.Photo
	var err error

	if p.CursorID != uuid.Nil {
		rows, err = r.store.Queries.ListUserPhotosAfterCursor(ctx, db.ListUserPhotosAfterCursorParams{
			UserID: p.OwnerID,
			CursorPhotoDate: pgtype.Date{
				Time:  p.CursorDate,
				Valid: true,
			},
			CursorID:   p.CursorID,
			LimitCount: p.Limit,
		})
	} else {
		rows, err = r.store.Queries.ListUserPhotosFirstPage(ctx, db.ListUserPhotosFirstPageParams{
			UserID:     p.OwnerID,
			LimitCount: p.Limit,
		})
	}

	if err != nil {
		return nil, err
	}

	return rows, nil
}

type photoByIDParams struct {
	photoID uuid.UUID
	ownerID uuid.UUID
}

func (r *usersRepository) photoByID(ctx context.Context, p photoByIDParams) (*db.GetPhotoByIDRow, error) {
	row, err := r.store.Queries.GetPhotoByID(ctx, db.GetPhotoByIDParams{UserID: p.ownerID, PhotoID: p.photoID})
	if err != nil {
		return nil, err
	}

	return &row, nil
}

type updateProfileParams struct {
	OwnerID     uuid.UUID
	DisplayName string
	About       string
	Visibility  string
}

func (r *usersRepository) updateProfile(ctx context.Context, p updateProfileParams) error {
	_, err := r.store.Queries.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
		DisplayName: p.DisplayName,
		About:       pgtype.Text{String: p.About, Valid: p.About != ""},
		Visibility:  p.Visibility,
		ID:          p.OwnerID,
	})
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}

	return nil
}

func (r *usersRepository) updateUsername(ctx context.Context, userID uuid.UUID, newUsername string) (*db.UpdateUsernameRow, error) {
	row, err := r.store.Queries.UpdateUsername(ctx, db.UpdateUsernameParams{ID: userID, NewUsernameKey: newUsername})
	if err != nil {
		return nil, fmt.Errorf("update username: %w", err)
	}

	return &row, nil
}

func (r *usersRepository) deleteProfile(ctx context.Context, userID uuid.UUID) error {
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin profile deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	q := r.store.Queries.WithTx(tx)
	if _, err := q.LockUserForDeletion(ctx, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lock profile for deletion: %w", err)
	}
	queued, err := q.QueueUserObjectsForDeletion(ctx, userID)
	if err != nil {
		return fmt.Errorf("queue profile objects for deletion: %w", err)
	}
	if err := q.DeleteProfile(ctx, userID); err != nil {
		return fmt.Errorf("delete profile record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit profile deletion: %w", err)
	}
	objects := make([]storage.Object, 0, len(queued))
	for _, job := range queued {
		objects = append(objects, storage.Object{Bucket: job.Bucket, Key: job.ObjectKey})
	}
	storage.DeleteNow(ctx, r.store, r.s3.Client, objects)
	return nil
}
