import { expect, openSettings, test } from "../fixtures.js";

test("Settings offers Log out only inside the mobile app", async ({ page }) => {
  await openSettings(page);
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true });
  await expect(dialog.getByRole("button", { name: "Done", exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Log out", exact: true })).toHaveCount(0);

  // The app's WebView injects agenttikApp into the server it opened.
  await page.addInitScript(() => {
    window.sent = [];
    window.agenttikApp = { postMessage: (message) => window.sent.push(message) };
  });
  await page.reload();
  await openSettings(page);
  const logout = page.waitForRequest((request) => new URL(request.url()).pathname === "/__auth/logout");
  await dialog.getByRole("button", { name: "Log out", exact: true }).click();
  await logout;
  await expect.poll(() => page.evaluate(() => window.sent)).toEqual(["disconnect"]);
});
