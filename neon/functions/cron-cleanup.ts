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

type PostgresError = Error & {
  code?: string;
  detail?: string;
};

const isPostgresError = (error: unknown): error is PostgresError =>
  error instanceof Error && ("code" in error || "detail" in error);

app.post("/session-cleanup", async (c) => {
  try {
    const invocation = await parseTrigger(c);

    const result = await pool.query(`
      DELETE FROM sessions
      WHERE absolute_expires_at <= now() - INTERVAL '3 days'
         OR revoked_at <= now() - INTERVAL '3 days'
    `);

    console.info("[cron]", {
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
      console.error("session cleanup failed", {
        message: error.message,
        code: error.code,
        detail: error.detail,
      });
    } else if (error instanceof Error) {
      console.error("session cleanup failed", { message: error.message });
    } else {
      console.error("session cleanup failed", { error });
    }

    return c.json({ ok: false, error: "session cleanup failed" }, 500);
  }
});

export default app;
