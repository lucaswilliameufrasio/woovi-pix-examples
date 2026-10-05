import { defineConfig } from "@playwright/test";

const baseURL = process.env.BROWSER_SMOKE_WEB_BASE;
if (!baseURL || !process.env.BROWSER_SMOKE_OUTPUT_DIR) {
  throw new Error(
    "Run browser tests through tooling/run_offers_smoke.py --browser.",
  );
}
const base = new URL(baseURL);
if (
  base.protocol !== "http:" ||
  base.hostname !== "127.0.0.1" ||
  !base.port ||
  base.username ||
  base.password ||
  base.pathname !== "/" ||
  base.search ||
  base.hash
) {
  throw new Error("Browser tests require an explicit loopback origin.");
}

export default defineConfig({
  testDir: "./tests/browser",
  outputDir: process.env.BROWSER_SMOKE_OUTPUT_DIR,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 45000,
  reporter: "line",
  use: {
    baseURL,
    browserName: "chromium",
    headless: true,
    // Operator authentication stays in the Node harness, not Chromium's env.
    launchOptions: {
      env: { PATH: process.env.PATH ?? "", HOME: process.env.HOME ?? "" },
    },
    trace: "off",
    screenshot: "off",
    video: "off",
  },
});
