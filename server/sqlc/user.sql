-- name: GetUserProfile :one
SELECT
  u.id,
  u.username_key,
  u.username_display,
  u.about,
  u.profile_visibility,
  u.created_at AS joined_at,
  a.bucket AS avatar_bucket,
  a.object_key AS avatar_object_key,
  (
    SELECT COUNT(p.id)
    FROM photos p
    WHERE p.owner_user_id = u.id
      AND p.status = 'ready'
      AND p.deleted_at IS NULL
  ) AS photo_count
FROM users u
LEFT JOIN user_avatars a ON u.id = a.user_id
WHERE u.username_key = sqlc.arg(username_key);

-- name: UpdateUserProfile :one
UPDATE users
SET username_display = sqlc.arg(display_name),
    about = sqlc.narg(about),
    profile_visibility = sqlc.arg(visibility),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING
  id,
  username_key,
  username_display,
  about,
  profile_visibility;

-- name: UpdateUsername :one
UPDATE users
SET username_key = sqlc.arg(new_username_key),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, username_key;

-- name: UpdatePasswordHash :one
UPDATE users
SET password_hash = sqlc.arg(new_password_hash),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
  AND password_hash = sqlc.arg(current_password_hash)
RETURNING id;

-- name: GetPasswordHash :one
SELECT id, username_key, password_hash FROM users
WHERE id = sqlc.arg(user_id);

-- name: CreateUser :one
INSERT INTO users (
  username_key, username_display, password_hash
) VALUES (
  sqlc.arg(username_key),
  sqlc.arg(username_display),
  sqlc.arg(password_hash)
)
RETURNING username_key, username_display;

-- name: GetUserDataForLogin :one
SELECT id, username_key, username_display, password_hash FROM users
WHERE username_key = sqlc.arg(username_key);

-- name: UsernameExists :one
SELECT EXISTS (
  SELECT 1
  FROM users
  WHERE username_key = sqlc.arg(username_key)
);

-- name: GetUserAccessProfile :one
SELECT id, profile_visibility
FROM users
WHERE username_key = sqlc.arg(username_key);

-- name: ListUserPhotosFirstPage :many
SELECT photos.*
FROM photos
WHERE owner_user_id = sqlc.arg(user_id)
  AND status = 'ready'
  AND deleted_at IS NULL
ORDER BY photo_date DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: ListUserPhotosAfterCursor :many
SELECT photos.*
FROM photos
WHERE owner_user_id = sqlc.arg(user_id)
  AND status = 'ready'
  AND deleted_at IS NULL
  AND (photo_date, id) < (sqlc.arg(cursor_photo_date)::date, sqlc.arg(cursor_id)::uuid)
ORDER BY photo_date DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: DeleteProfile :exec
DELETE FROM users WHERE id = sqlc.arg(user_id);

-- name: LockUserForDeletion :one
SELECT id FROM users WHERE id = sqlc.arg(user_id) FOR UPDATE;

-- name: GetPhotoByID :one
WITH current_photo AS (
  SELECT
    p.id,
    p.owner_user_id,
    p.bucket,
    p.object_key_original,
    p.title,
    p.description,
    p.photo_date
  FROM photos AS p
  WHERE p.id = sqlc.arg(photo_id)
    AND p.owner_user_id = sqlc.arg(user_id)
    AND p.status = 'ready'
    AND p.deleted_at IS NULL
)
SELECT
  current_photo.*,

  (
    SELECT p.id
    FROM photos p
    WHERE p.owner_user_id = sqlc.arg(user_id)
      AND p.status = 'ready'
      AND p.deleted_at IS NULL
      AND (p.photo_date, p.id) > (current_photo.photo_date, current_photo.id)
    ORDER BY p.photo_date ASC, p.id ASC
    LIMIT 1
  ) AS previous_photo_id,

  (
    SELECT p.id
    FROM photos p
    WHERE p.owner_user_id = sqlc.arg(user_id)
      AND p.status = 'ready'
      AND p.deleted_at IS NULL
      AND (p.photo_date, p.id) < (current_photo.photo_date, current_photo.id)
    ORDER BY p.photo_date DESC, p.id DESC
    LIMIT 1
  ) AS next_photo_id
FROM current_photo;
