// This file was AI-generated to centralize immediate storage deletion and
// deletion-job completion, based on the project's existing deletion flows.

package storage

import (
	"context"
	"fmt"
	"lensamity/internal/db"

	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Object struct {
	Bucket string
	Key    string
}

// DeleteNow attempts storage cleanup after the database transaction commits.
// Queued jobs remain available for retry if storage or database completion fails.
func DeleteNow(ctx context.Context, store *db.Store, client *s3.Client, objects []Object) {
	if len(objects) == 0 {
		return
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	byBucket := make(map[string][]string)
	for _, object := range objects {
		byBucket[object.Bucket] = append(byBucket[object.Bucket], object.Key)
	}

	for bucket, keys := range byBucket {
		for start := 0; start < len(keys); start += 1000 {
			end := min(start+1000, len(keys))
			batch := keys[start:end]
			if err := deleteBatch(cleanupCtx, store, client, bucket, batch); err != nil {
				slog.Warn("immediate storage cleanup failed; jobs remain queued", "bucket", bucket, "error", err)
			}
		}
	}
}

func deleteBatch(ctx context.Context, store *db.Store, client *s3.Client, bucket string, keys []string) error {
	if len(keys) == 1 {
		key := keys[0]
		_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return fmt.Errorf("delete object %q: %w", key, err)
		}
		return completeJobIfDue(ctx, store, bucket, key)
	}

	identifiers := make([]types.ObjectIdentifier, 0, len(keys))
	for _, key := range keys {
		identifiers = append(identifiers, types.ObjectIdentifier{Key: aws.String(key)})
	}

	result, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(bucket),
		Delete: &types.Delete{Objects: identifiers, Quiet: aws.Bool(false)},
	})
	if err != nil {
		return fmt.Errorf("delete objects: %w", err)
	}

	for _, item := range result.Errors {
		slog.Warn("immediate object deletion failed; job remains queued",
			"bucket", bucket, "key", aws.ToString(item.Key),
			"code", aws.ToString(item.Code), "message", aws.ToString(item.Message))
	}
	for _, item := range result.Deleted {
		if item.Key == nil {
			continue
		}
		if err := completeJobIfDue(ctx, store, bucket, *item.Key); err != nil {
			slog.Warn("storage object deleted but job completion failed", "bucket", bucket, "key", *item.Key, "error", err)
		}
	}
	return nil
}

func completeJobIfDue(ctx context.Context, store *db.Store, bucket, key string) error {
	if err := store.Queries.CompleteStorageDeletionJobIfDue(ctx, db.CompleteStorageDeletionJobIfDueParams{
		Bucket: bucket, ObjectKey: key,
	}); err != nil {
		return fmt.Errorf("complete storage deletion job: %w", err)
	}
	return nil
}
