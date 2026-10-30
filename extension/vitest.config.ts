import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    include: ["tests/unit/**/*.test.ts"],
    coverage: {
      provider: "v8",
      include: ["src/**/*.ts"],
      // content.ts / sidepanel.ts are thin chrome-wiring entrypoints exercised by
      // the Playwright e2e, not unit tests; the pure logic they call is covered.
      exclude: ["src/content.ts", "src/sidepanel.ts", "src/types.ts"],
      thresholds: {
        statements: 90,
        branches: 85,
        functions: 90,
        lines: 90,
      },
      reporter: ["text", "html"],
    },
  },
});
