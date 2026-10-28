-- +goose Up
CREATE TABLE storage_deletion_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bucket VARCHAR(63) NOT NULL,
  object_key TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  claimed_until TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,

  CONSTRAINT storage_deletion_jobs_bucket_not_empty
    CHECK (length(bucket) > 0),
  CONSTRAINT storage_deletion_jobs_object_key_not_empty
    CHECK (length(object_key) > 0),
  CONSTRAINT storage_deletion_jobs_attempts_nonnegative
    CHECK (attempts >= 0)
);

CREATE UNIQUE INDEX storage_deletion_jobs_pending_object_unique
  ON storage_deletion_jobs (bucket, object_key)
  WHERE completed_at IS NULL;

CREATE INDEX storage_deletion_jobs_due_idx
  ON storage_deletion_jobs (next_attempt_at, id)
  WHERE completed_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS storage_deletion_jobs;
