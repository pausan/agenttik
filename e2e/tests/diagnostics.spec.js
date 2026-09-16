import { expect, openSettings, test } from "../fixtures.js";

test("About keeps safe errors across reloads and copies and clears reports", async ({ page, context }) => {
  await openSettings(page, "About");
  await expect(page.getByText("No errors recorded.")).toBeVisible();
  await page.evaluate(() => {
    const error = new TypeError("secret prompt person@example.com");
    window.dispatchEvent(new ErrorEvent("error", { error }));
  });
  await expect(page.getByText("TypeError · runtime")).toBeVisible();
  await page.reload();
  await openSettings(page, "About");
  await expect(page.getByText("TypeError · runtime")).toBeVisible();
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.getByRole("button", { name: "Copy all errors", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Copied report" })).toBeVisible();
  const report = await page.evaluate(() => navigator.clipboard.readText());
  expect(JSON.parse(report).errors[0].kind).toBe("TypeError");
  expect(report).not.toMatch(/secret|person@example/);
  await page.getByRole("button", { name: "Clear errors", exact: true }).click();
  await expect(page.getByText("No errors recorded.")).toBeVisible();
});
