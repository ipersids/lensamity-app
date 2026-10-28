import { DeleteObjectsCommand, S3Client } from "@aws-sdk/client-s3";
import { parseEnv } from "@neon/env";
import { attachDatabasePool } from "@neon/functions";
import { parseTrigger } from "@neon/functions/hono";
import { Hono } from "hono";
import { Pool } from "pg";
import neonConfig from "../../neon.js";

const app = new Hono();
const pool = new Pool({
  connectionString: parseEnv(neonConfig, ["DATABASE_URL"]).postgres.databaseUrl,
  max: 3,
});
attachDatabasePool(pool);
const client = new S3Client({
  region: parseEnv(neonConfig, ["AWS_REGION"]).storage.region,
  endpoint: parseEnv(neonConfig, ["AWS_ENDPOINT_URL_S3"]).storage.endpoint,
  credentials: {
    accessKeyId: parseEnv(neonConfig, ["AWS_ACCESS_KEY_ID"]).storage.accessKeyId,
    secretAccessKey: parseEnv(neonConfig, ["AWS_SECRET_ACCESS_KEY"]).storage.secretAccessKey,
  },
  forcePathStyle: true,
  requestChecksumCalculation: "WHEN_REQUIRED",
});

type PostgresError = Error & {
  code?: string;
  detail?: string;
};

const isPostgresError = (error: unknown): error is PostgresError =>
  error instanceof Error && ("code" in error || "detail" in error);

type StorageDeletionJob = {
  id: string;
  bucket: string;
  object_key: string;
  claimed_until: string;
};

type DeletionOutcome = {
  id: string;
  claimed_until: string;
  error: string | null;
};

app.post("/session-cleanup", async (c) => {
  try {
    const invocation = await parseTrigger(c);

    const result = await pool.query(`
      DELETE FROM sessions
      WHERE absolute_expires_at <= now() - INTERVAL '3 days'
         OR revoked_at <= now() - INTERVAL '3 days'
    `);

    console.info("[ cron ] storage cleanup completed", {
      invocationId: invocation.invocationId,
      triggerName: invocation.trigger.name,
      triggerID: invocation.trigger.id,
      scheduledAt: invocation.data.scheduledAt,
      deleted: result.rowCount ?? 0,
    });

    return c.json({
      ok: true,
      scheduled_at: invocation.data.scheduledAt,
      deleted: result.rowCount ?? 0,
    });
  } catch (error: unknown) {
    if (isPostgresError(error)) {
      console.error("[ cron ] session cleanup failed", {
        message: error.message,
        code: error.code,
        detail: error.detail,
      });
    } else if (error instanceof Error) {
      console.error("[ cron ] session cleanup failed", { message: error.message });
    } else {
      console.error("[ cron ] session cleanup failed", { error });
    }

    return c.json({ ok: false, error: "[ cron ] session cleanup failed" }, 500);
  }
});

app.post("/storage-cleanup", async (c) => {
  try {
    const invocation = await parseTrigger(c);
    let deleted = 0;
    let failed = 0;

    while (true) {
      const jobs = await pool.query<StorageDeletionJob>(`
        WITH due AS (
          SELECT id
          FROM storage_deletion_jobs
          WHERE completed_at IS NULL
            AND next_attempt_at <= now()
            AND (claimed_until IS NULL OR claimed_until < now())
          ORDER BY next_attempt_at, id
          LIMIT 100
          FOR UPDATE SKIP LOCKED
        )
        UPDATE storage_deletion_jobs AS j
        SET claimed_until = now() + interval '1 hour',
            attempts = attempts + 1
        FROM due
        WHERE j.id = due.id
        -- Preserve full timestamp precision for the lease comparison below.
        RETURNING j.id, j.bucket, j.object_key,
                  j.claimed_until::text AS claimed_until;
      `);

      if (!jobs.rowCount || jobs.rowCount === 0) {
        console.info("[cron] storage cleanup completed", {
          invocationId: invocation.invocationId,
          triggerName: invocation.trigger.name,
          triggerID: invocation.trigger.id,
          scheduledAt: invocation.data.scheduledAt,
          deleted,
          failed,
        });

        return c.json({ ok: true, deleted, failed });
      }

      const bucketMap: Record<string, StorageDeletionJob[]> = {};

      for (const row of jobs.rows) {
        const bucketJobs = bucketMap[row.bucket];
        if (bucketJobs) {
          bucketJobs.push(row);
        } else {
          bucketMap[row.bucket] = [row];
        }
      }

      for (const [bucket, bucketJobs] of Object.entries(bucketMap)) {
        let outcomes: DeletionOutcome[];

        try {
          const result = await client.send(
            new DeleteObjectsCommand({
              Bucket: bucket,
              Delete: {
                Objects: bucketJobs.map((job) => ({ Key: job.object_key })),
              },
            }),
          );

          const deletedKeys = new Set(result.Deleted?.map((item) => item.Key) ?? []);
          const failedKeys = new Map(
            (result.Errors ?? [])
              .filter((item) => item.Key !== undefined)
              .map(
                (item) => [item.Key, `${item.Code ?? "Unknown"}: ${item.Message ?? ""}`] as const,
              ),
          );

          outcomes = bucketJobs.map((job) => {
            const failure = failedKeys.get(job.object_key);
            return {
              id: job.id,
              claimed_until: job.claimed_until,
              error:
                failure ??
                (deletedKeys.has(job.object_key) ? null : "No deletion result returned by storage"),
            };
          });
        } catch (error: unknown) {
          const message = error instanceof Error ? error.message : String(error);
          outcomes = bucketJobs.map((job) => ({
            id: job.id,
            claimed_until: job.claimed_until,
            error: message,
          }));
        }

        const updated = await pool.query<{
          id: string;
          completed_at: Date | null;
        }>(
          `UPDATE storage_deletion_jobs AS j
           SET completed_at = CASE WHEN outcome.error IS NULL THEN now() ELSE j.completed_at END,
               next_attempt_at = CASE
                 WHEN outcome.error IS NOT NULL THEN now() + interval '12 hour'
                 ELSE j.next_attempt_at
               END,
               last_error = outcome.error,
               claimed_until = NULL
           FROM jsonb_to_recordset($1::jsonb)
             AS outcome(id uuid, claimed_until timestamptz, error text)
           WHERE j.id = outcome.id
             AND j.claimed_until = outcome.claimed_until
             AND j.completed_at IS NULL
           RETURNING j.id, j.completed_at`,
          [JSON.stringify(outcomes)],
        );

        for (const row of updated.rows) {
          if (row.completed_at === null) {
            failed++;
          } else {
            deleted++;
          }
        }

        if (updated.rowCount !== outcomes.length) {
          console.warn("[ cron ] storage cleanup lost claims", {
            bucket,
            claimed: outcomes.length,
            updated: updated.rowCount ?? 0,
          });
        }
      }
    }
  } catch (error: unknown) {
    console.error("[ cron ] storage cleanup failed", error);
    return c.json({ ok: false, error: "storage cleanup failed" }, 500);
  }
});

export default app;
