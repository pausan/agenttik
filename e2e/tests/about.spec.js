import { expect, openSettings, test } from "../fixtures.js";

test("About shows the running version, author and license and can be found by filtering", async ({ page }) => {
  await openSettings(page, "About");
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true });
  await expect(dialog.locator("nav button").nth(-2)).toHaveText("Help");
  await expect(dialog.locator("nav button").nth(-1)).toHaveText("About");
  const info = await page.evaluate(() => fetch("/api/version").then((response) => response.json()));
  await expect(dialog.getByText(`Version ${info.version}`, { exact: true })).toBeVisible();
  await expect(dialog.getByText("Created by Pau Sánchez", { exact: true })).toBeVisible();
  await expect(dialog.getByText("Copyright © 2026 Pau Sánchez", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("heading", { name: "MIT License", exact: true })).toBeVisible();
  await expect(dialog.getByText("Permission is hereby granted", { exact: false })).toBeVisible();
  await dialog.getByRole("button", { name: "General", exact: true }).click();
  await dialog.getByPlaceholder("Filter settings…").fill("Pau Sánchez");
  await expect(dialog.getByRole("heading", { name: "About agenttik" })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "About 1", exact: true })).toBeVisible();
});

test("Troubleshooting is part of About, not Help", async ({ page }) => {
  await openSettings(page, "Help");
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true });
  await expect(dialog.getByText("Last Errors", { exact: true })).toBeHidden();
  await dialog.getByRole("button", { name: "About", exact: true }).click();
  await expect(dialog.getByText("Last Errors", { exact: true })).toBeVisible();
});
