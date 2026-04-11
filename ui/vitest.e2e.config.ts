import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["e2e/**/*.test.{js,ts}"],
    exclude: ["e2e/**/*.spec.ts"], // Exclude Playwright spec files
    environment: "node",
    globals: true,
  },
});
