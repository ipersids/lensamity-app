-- name: QueuePhotoObjectForDeletion :exec
INSERT INTO storage_deletion_jobs (bucket, object_key, next_attempt_at)
SELECT p.bucket,
       p.object_key_original,
       GREATEST(now(), p.expires_at + interval '5 minutes')
FROM photos AS p
WHERE p.id = sqlc.arg(photo_id)
ON CONFLICT (bucket, object_key) WHERE completed_at IS NULL
DO UPDATE SET next_attempt_at = GREATEST(
  storage_deletion_jobs.next_attempt_at,
  EXCLUDED.next_attempt_at
);

-- name: QueueAvatarObjectForDeletion :exec
INSERT INTO storage_deletion_jobs (bucket, object_key, next_attempt_at)
SELECT a.bucket,
       a.object_key,
       GREATEST(now(), a.expires_at + interval '5 minutes')
FROM avatar_uploads AS a
WHERE a.id = sqlc.arg(avatar_upload_id)
ON CONFLICT (bucket, object_key) WHERE completed_at IS NULL
DO UPDATE SET next_attempt_at = GREATEST(
  storage_deletion_jobs.next_attempt_at,
  EXCLUDED.next_attempt_at
);

-- name: QueueUserObjectsForDeletion :many
WITH objects AS (
  SELECT bucket, object_key_original AS object_key, expires_at
  FROM photos
  WHERE owner_user_id = sqlc.arg(user_id)

  UNION ALL

  SELECT bucket, object_key, expires_at
  FROM avatar_uploads
  WHERE user_id = sqlc.arg(user_id)

  UNION ALL

  SELECT bucket, object_key, now() AS expires_at
  FROM user_avatars
  WHERE user_id = sqlc.arg(user_id)
), due AS (
  SELECT bucket,
         object_key,
         GREATEST(now(), max(expires_at) + interval '5 minutes') AS next_attempt_at
  FROM objects
  GROUP BY bucket, object_key
)
INSERT INTO storage_deletion_jobs (bucket, object_key, next_attempt_at)
SELECT bucket, object_key, next_attempt_at
FROM due
ON CONFLICT (bucket, object_key) WHERE completed_at IS NULL
DO UPDATE SET next_attempt_at = GREATEST(
  storage_deletion_jobs.next_attempt_at,
  EXCLUDED.next_attempt_at
)
RETURNING bucket, object_key;

-- name: CompleteStorageDeletionJobIfDue :exec
UPDATE storage_deletion_jobs
SET completed_at = now(), claimed_until = NULL, last_error = NULL
WHERE bucket = sqlc.arg(bucket)
  AND object_key = sqlc.arg(object_key)
  AND completed_at IS NULL
  AND claimed_until IS NULL
  AND next_attempt_at <= now();
