import { defineConfig, devices } from "@playwright/test";

/* Tests bring their own server: fixtures.js starts an agenttik per test, on
   its own port over its own database, so they can run in parallel and never
   see each other's projects. `make e2e` builds the binary first. */
/* Tests that hold the app to a time budget: frame, load and render times.
   Under a full parallel run they measure the contention, not the app, so they
   have a project of their own, one test at a time. `make e2e` runs it after
   the rest. */
const timed = /(performance|speed|large-files|startup)\.spec\.js$/;

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "list" : [["list"], ["html", { open: "never" }]],
  // A full parallel run starts up to one server per test at once; the
  // default 5s per assertion is occasionally too tight for the slowest of
  // them under that contention, not because anything is actually stuck.
  expect: { timeout: 8_000 },
  use: {
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      testIgnore: timed,
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
    {
      name: "timed",
      testMatch: timed,
      workers: 1,
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
  ],
});
