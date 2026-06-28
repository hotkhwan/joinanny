import { defineConfig, devices } from "@playwright/test";

// E2E config: Playwright launches the Mongo-free e2eserver (cmd/e2eserver),
// which serves the real dashboard + API with in-memory stores and a stub
// Binance backend, then drives it in a headless browser. No network, no DB.
const PORT = process.env.E2E_PORT || "8099";
const BASE_URL = `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL: BASE_URL,
    headless: true,
    trace: "retain-on-failure",
  },
  webServer: {
    command: "go run ./cmd/e2eserver",
    cwd: "..",
    env: { E2E_ADDR: `:${PORT}` },
    url: `${BASE_URL}/healthz`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
