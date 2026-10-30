import { defineConfig } from "@playwright/test";

// E2e runs against the built extension in dist/ (pnpm e2e runs `build` first).
// Chromium loads MV3 extensions only with a persistent context, so the spec
// launches its own context; this config just sets sane defaults + a fixture web
// server for pages to capture.
export default defineConfig({
  testDir: "tests/e2e",
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? "list" : [["list"]],
  use: {
    trace: "on-first-retry",
  },
  webServer: {
    command: "node tests/e2e/fixture-server.mjs",
    url: "http://localhost:5173/github-pr.html",
    reuseExistingServer: !process.env.CI,
    stdout: "ignore",
  },
});
