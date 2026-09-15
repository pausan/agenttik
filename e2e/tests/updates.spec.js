import { expect, test } from "../fixtures.js";

async function mockUpdates(page, onInstall) {
  let status = { available: { version: "v99.0.0" }, busy: false, ready: false };
  await page.route("**/api/updates**", async (route) => {
    const request = route.request();
    if (request.method() === "POST") {
      expect(request.postDataJSON()).toEqual({ version: "v99.0.0" });
      if (request.url().endsWith("/ignore")) status = {};
      else if (onInstall) return onInstall(route);
      else status = { ...status, ready: true };
    }
    await route.fulfill({ json: status });
  });
  // The fixture has already navigated; reload with interception in place.
  await page.reload();
}

test("ignore hides a release and stays hidden after reload", async ({ page }) => {
  await mockUpdates(page);
  await expect(page.getByText("Agenttik v99.0.0 is available")).toBeVisible();
  await page.getByRole("button", { name: "Ignore this version", exact: true }).click();
  await expect(page.getByText("Agenttik v99.0.0 is available")).toHaveCount(0);
  await page.reload();
  await page.waitForTimeout(3500);
  await expect(page.getByText("Agenttik v99.0.0 is available")).toHaveCount(0);
});

test("update explains installation on quit", async ({ page }) => {
  await mockUpdates(page);
  await page.getByRole("button", { name: "Update", exact: true }).click();
  await expect(page.getByText("Update ready", { exact: true })).toBeVisible();
  await expect(page.getByText(/Quit Agenttik to install, then reopen it/)).toBeVisible();
  await page.getByRole("button", { name: "Dismiss", exact: true }).click();
  await expect(page.getByText("Update ready", { exact: true })).toHaveCount(0);
});

test("failed installation displays the error and allows retry", async ({ page }) => {
  await mockUpdates(page, (route) => route.fulfill({ status: 409, json: { error: "Administrator approval was cancelled" } }));
  await page.getByRole("button", { name: "Update", exact: true }).click();
  await expect(page.getByText("Administrator approval was cancelled", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Update", exact: true })).toBeEnabled();
});
