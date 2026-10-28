-- name: CreatePendingPhotoUploadRecord :exec
INSERT INTO photos (
  id,
  owner_user_id,
  bucket,
  object_key_original,
  content_type,
  size,
  photo_date,
  title,
  description,
  expires_at
) VALUES (
  sqlc.arg(id),
  sqlc.arg(owner_user_id),
  sqlc.arg(bucket),
  sqlc.arg(object_key_original),
  sqlc.arg(content_type),
  sqlc.arg(size),
  sqlc.arg(photo_date),
  sqlc.arg(title),
  sqlc.arg(description),
  sqlc.arg(expires_at)
);

-- name: MarkPhotoUploadRecordProcessing :one
UPDATE photos
  SET status = 'processing'
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND status = 'pending'
RETURNING
  id,
  owner_user_id,
  bucket,
  object_key_original,
  content_type,
  size,
  photo_date,
  status;

-- name: MarkProcessedPhotoUploadRecordCompleted :one
UPDATE photos
  SET status = 'ready',
      uploaded_at = now(),
      processed_at = now(),
      object_key_processed = sqlc.arg(object_key_processed),
      width = sqlc.arg(width),
      height = sqlc.arg(height)
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND status = 'processing'
RETURNING id;

-- name: MarkProcessedPhotoUploadRecordFailed :one
UPDATE photos
SET status = 'failed',
    failure_reason = sqlc.arg(failure_reason)
WHERE id = sqlc.arg(id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND status = 'processing'
RETURNING bucket, object_key_original;

-- name: MarkPhotoRecordDeleted :one
UPDATE photos
  SET status = 'deleted',
      deleted_at = now()
WHERE id = sqlc.arg(photo_id)
  AND owner_user_id = sqlc.arg(owner_user_id)
  AND deleted_at IS NULL
RETURNING id, bucket, object_key_original;

-- name: CreatePendingAvatarUpload :exec
INSERT INTO avatar_uploads (
  id,
  user_id,
  bucket,
  object_key,
  content_type,
  size,
  expires_at
) VALUES (
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(bucket),
  sqlc.arg(object_key),
  sqlc.arg(content_type),
  sqlc.arg(size),
  sqlc.arg(expires_at)
);

-- name: MarkAvatarUploadProcessing :one
UPDATE avatar_uploads
SET status = 'processing'
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND status IN ('pending', 'processing')
  AND expires_at > now()
RETURNING id, user_id, bucket, object_key, content_type, size, status;

-- name: GetAvatarUploadStatus :one
SELECT status
FROM avatar_uploads
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id);

-- name: MarkAvatarUploadFailed :one
UPDATE avatar_uploads
SET status = 'failed', failure_reason = sqlc.arg(failure_reason)
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'processing'
RETURNING bucket, object_key;

-- name: GetCurrentAvatarForUpdate :one
SELECT user_id, bucket, object_key, content_type, uploaded_at, size
FROM user_avatars
WHERE user_id = sqlc.arg(user_id)
FOR UPDATE;

-- name: MarkAvatarUploadReady :exec
UPDATE avatar_uploads
SET status = 'ready', completed_at = now()
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'processing';

-- name: MarkAvatarUploadSuperseded :exec
UPDATE avatar_uploads
SET status = 'superseded', superseded_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bucket = sqlc.arg(bucket)
  AND object_key = sqlc.arg(object_key)
  AND status = 'ready';

-- name: MarkAvatarUploadDeleted :exec
UPDATE avatar_uploads
SET status = 'deleted', deleted_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND bucket = sqlc.arg(bucket)
  AND object_key = sqlc.arg(object_key)
  AND status IN ('ready', 'superseded');

-- name: UpsertCurrentAvatar :exec
INSERT INTO user_avatars (user_id, bucket, object_key, content_type, size)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(bucket),
  sqlc.arg(object_key),
  sqlc.arg(content_type),
  sqlc.arg(size)
)
ON CONFLICT (user_id) DO UPDATE
SET bucket = EXCLUDED.bucket,
    object_key = EXCLUDED.object_key,
    content_type = EXCLUDED.content_type,
    size = EXCLUDED.size,
    uploaded_at = now();

-- name: DeleteCurrentAvatar :one
DELETE FROM user_avatars
WHERE user_id = sqlc.arg(user_id)
RETURNING bucket, object_key;
