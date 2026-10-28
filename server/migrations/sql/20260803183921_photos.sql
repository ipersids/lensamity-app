-- +goose Up
CREATE TYPE upload_status AS ENUM (
  'pending',
  'processing',
  'ready',
  'failed',
  'expired',
  'deleted'
);

CREATE TABLE IF NOT EXISTS photos (
  id UUID PRIMARY KEY,
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status upload_status NOT NULL DEFAULT 'pending',
  failure_reason TEXT,

  bucket VARCHAR(63) NOT NULL,
  object_key_original TEXT NOT NULL UNIQUE,

  content_type TEXT NOT NULL,
  size BIGINT NOT NULL,
  width INTEGER,
  height INTEGER,

  photo_date DATE NOT NULL,
  title TEXT,
  description TEXT,

  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  uploaded_at TIMESTAMPTZ,
  processed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ,

  CONSTRAINT photos_object_key_original_not_empty
    CHECK (length(object_key_original) > 0),
  CONSTRAINT photos_allowed_content_type
    CHECK (
      content_type IN (
        'image/jpeg',
        'image/png',
        'image/webp'
      )
    ),
  CONSTRAINT photos_size_positive
    CHECK (size > 0),
  CONSTRAINT photos_dimensions_positive
    CHECK (
      (width IS NULL AND height IS NULL)
      OR (width > 0 AND height > 0)
    ),
  CONSTRAINT photos_title_length
    CHECK (title IS NULL OR length(title) <= 120),
  CONSTRAINT photos_description_length
    CHECK (description IS NULL OR length(description) <= 2000)
);

CREATE UNIQUE INDEX IF NOT EXISTS photos_owner_photo_date_active_unique
ON photos (owner_user_id, photo_date)
WHERE deleted_at IS NULL
  AND status IN ('pending', 'processing', 'ready');

CREATE INDEX IF NOT EXISTS photos_expirable_idx
ON photos (expires_at)
WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS photos;
DROP TYPE IF EXISTS upload_status;
