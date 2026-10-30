import { defineConfig } from "@playwright/test";

// E2e runs against the built extension in dist/ (pnpm e2e runs `build` first).
// Chromium loads MV3 extensions only with a persistent context, so the spec
// launches its own context; this config just sets sane defaults (fixture pages are
// served via page.route).
export default defineConfig({
  testDir: "tests/e2e",
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? "list" : [["list"]],
  use: {
    trace: "on-first-retry",
  },
});
