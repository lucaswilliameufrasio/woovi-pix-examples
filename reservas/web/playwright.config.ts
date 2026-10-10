import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/browser",
  timeout: 30_000,
  expect: { timeout: 8_000 },
  use: { baseURL: process.env.BROWSER_SMOKE_BASE_URL, browserName: "chromium" },
  reporter: "list",
});
