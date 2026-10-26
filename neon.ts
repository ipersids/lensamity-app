import { defineConfig } from "@neon/config/v1";

export default defineConfig({
  auth: false,

  functions: {
    cleanup: {
      name: "Cron cleanup jobs",
      source: "neon/functions/cron-cleanup.ts",
      dev: { port: 8787 },
    },
  },
  triggers: {
    session: {
      type: "schedule",
      function: "cleanup",
      functionPath: "/session-cleanup",
      cron: "0 3 * * *",
    },
  },

  // Branch policy: per-branch tuning
  branch: (branch) => {
    if (branch.isDefault) {
      // Default branch: no overrides, uses project defaults
      return {};
    }
    if (!branch.exists) {
      // New non-default branches: auto-expire
      // Run `neon checkout <name> --env --file <.env.name>` to create a new branch with these settings
      return { ttl: "1d" };
    }
    // Existing branch: no changes
    return {};
  },
});
