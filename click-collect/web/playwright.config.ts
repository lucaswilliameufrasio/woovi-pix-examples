import { defineConfig } from "@playwright/test";

const baseURL = process.env.CLICK_COLLECT_WEB_BASE_URL;
if (!baseURL || !process.env.BROWSER_SMOKE_OUTPUT_DIR) {
  throw new Error(
    "Use tooling/run_click_collect_smoke.py --browser to run browser tests.",
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
  timeout: 45_000,
  reporter: "line",
  use: {
    baseURL,
    browserName: "chromium",
    headless: true,
    launchOptions: {
      env: { PATH: process.env.PATH ?? "", HOME: process.env.HOME ?? "" },
    },
    trace: "off",
    screenshot: "off",
    video: "off",
  },
});
