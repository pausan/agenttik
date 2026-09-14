import { expect, openSettings, test } from "../fixtures.js";

test("appearance modes persist and System follows device changes", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await openSettings(page, "Appearance");
  const mode = page.getByRole("button", { name: "Color mode", exact: true });
  const root = page.locator("html");
  const choose = async (name) => {
    await mode.click();
    await page.getByRole("option", { name, exact: true }).click();
    await expect(mode).toContainText(name);
  };
  await expect(mode).toContainText("System");
  await expect(root).not.toHaveClass(/dark/);
  const lightBackground = await page.locator("body").evaluate((el) => getComputedStyle(el).backgroundColor);
  await choose("Dark");
  await expect(root).toHaveClass(/dark/);
  await expect(root).toHaveCSS("color-scheme", "dark");
  expect(await page.locator("body").evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe(lightBackground);
  await page.reload();
  await expect(root).toHaveClass(/dark/);
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("Dark");
  await page.emulateMedia({ colorScheme: "dark" });
  await choose("Light");
  await expect(root).not.toHaveClass(/dark/);
  await expect(root).toHaveCSS("color-scheme", "light");
  await page.reload();
  await expect(root).not.toHaveClass(/dark/);
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("Light");
  await choose("System");
  await expect(root).toHaveClass(/dark/);
  await page.emulateMedia({ colorScheme: "light" });
  await expect(root).not.toHaveClass(/dark/);
  await page.emulateMedia({ colorScheme: "dark" });
  await expect(root).toHaveClass(/dark/);
  await page.reload();
  await expect(root).toHaveClass(/dark/);
  await openSettings(page, "Appearance");
  await expect(mode).toContainText("System");
  await page.getByPlaceholder("Filter settings…").fill("night");
  await expect(mode).toBeVisible();
  await expect(page.getByRole("button", { name: "Appearance 1", exact: true })).toBeVisible();
});

test("General disappears when another settings section is selected", async ({ page }) => {
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  const prompt = dialog.getByText("What the prompt box does", { exact: false });
  const folding = dialog.getByText("What selecting a project does", { exact: false });
  await expect(prompt).toBeVisible();
  await expect(folding).toBeVisible();
  for (const section of ["Projects", "Orchestrator", "Appearance", "Models", "Subscriptions", "Server", "Shortcuts"]) {
    await dialog.getByRole("button", { name: section, exact: true }).click();
    await expect(prompt).toBeHidden();
    await expect(folding).toBeHidden();
  }
  await dialog.getByRole("button", { name: "General", exact: true }).click();
  await expect(prompt).toBeVisible();
  await expect(folding).toBeVisible();
});

test("authentication enables before a password and the current code refreshes", async ({ page }) => {
  const config = { available: true, enabled: false, host: "127.0.0.1", port: 7717,
    auth_enabled: false, has_password: false, totp_secret: "" };
  await page.route("**/api/server", (route) => route.fulfill({ json: config }));
  await page.route("**/api/server/auth", async (route) => {
    const body = route.request().postDataJSON();
    expect(body.password).toBe("");
    config.auth_enabled = body.enabled;
    config.totp_secret = "GEZDGNBVGY3TQOJQ";
    await route.fulfill({ json: config });
  });
  await page.route("**/api/server/auth/totp.png?*", (route) => route.fulfill({ status: 204 }));
  let reads = 0;
  await page.route("**/api/server/auth/code", (route) => route.fulfill({
    json: { code: ++reads === 1 ? "123456" : "654321", refresh_after_ms: 30000 },
  }));
  await page.clock.install();
  await openSettings(page, "Server");
  const dialog = page.getByRole("dialog");
  const toggle = dialog.getByRole("switch", { name: "Ask for a password and a code" });
  await toggle.click();
  await expect(toggle).toBeChecked();
  await expect(dialog.getByText("Browser access is blocked", { exact: false })).toBeVisible();
  await expect(dialog.getByLabel("Authenticator seed")).toHaveValue(config.totp_secret);
  await expect(dialog.getByLabel("Current code")).toHaveText("123456");
  await page.clock.fastForward(30000);
  await expect(dialog.getByLabel("Current code")).toHaveText("654321");
  await dialog.getByRole("button", { name: "General", exact: true }).click();
  const previous = reads;
  await page.clock.fastForward(60000);
  expect(reads).toBe(previous);
  await dialog.getByRole("button", { name: "Server", exact: true }).click();
  await expect(dialog.getByLabel("Current code")).toHaveText("654321");
  await toggle.click();
  await expect(toggle).not.toBeChecked();
  await expect(dialog.getByLabel("Current code")).toBeHidden();
});

test("new item position defaults to top and remembers bottom", async ({ page }) => {
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  const top = dialog.getByRole("radio", { name: "At the top (default)", exact: true });
  const bottom = dialog.getByRole("radio", { name: "At the bottom", exact: true });
  await expect(top).toBeChecked();
  await expect(bottom).toBeEnabled();
  await bottom.click();
  await expect(bottom).toBeChecked();
  await page.reload();
  await openSettings(page);
  await expect(bottom).toBeChecked();
  await top.click();
  await expect(top).toBeChecked();
});


test("desktop tray settings save the toggle and shortcut", async ({ page }) => {
  let config = { available: true, close_to_tray: false, toggle_shortcut: "Ctrl+Shift+A", error: "" };
  await page.route("**/api/desktop", async (route) => {
    if (route.request().method() === "PUT") config = { ...config, ...route.request().postDataJSON() };
    await route.fulfill({ json: config });
  });
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Show / hide shortcut")).toHaveValue("Ctrl+Shift+A");
  await dialog.getByRole("checkbox", { name: "Close to tray" }).check();
  await dialog.getByLabel("Show / hide shortcut").fill("Alt+Shift+B");
  await dialog.getByRole("button", { name: "Save tray settings" }).click();
  await expect(dialog.getByText("Saved. Restart agenttik to apply these settings.")).toBeVisible();
  expect(config.close_to_tray).toBe(true);
  expect(config.toggle_shortcut).toBe("Alt+Shift+B");
  await page.reload();
  await openSettings(page);
  await expect(dialog.getByRole("checkbox", { name: "Close to tray" })).toBeChecked();
  await expect(dialog.getByLabel("Show / hide shortcut")).toHaveValue("Alt+Shift+B");
});

test("web mode does not offer tray controls", async ({ page }) => {
  await openSettings(page);
  await expect(page.getByText("Desktop tray", { exact: true })).toHaveCount(0);
});


test("database path is read-only, copyable, and searchable with a formatted size", async ({ page, context, agenttik }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  const path = dialog.getByLabel("Path on the Agenttik host");
  await expect(path).toHaveText(`${agenttik.dataDir}/agenttik.db`);
  await expect(path).toHaveClass(/bg-muted/);
  await expect(dialog.locator("section:visible > div.font-semibold").filter({ hasText: /^Database$/ })).toBeVisible();
  await expect(dialog.getByText(/^Database file size: \d+\.\d (B|KB|MB|GB)$/)).toBeVisible();
  await dialog.getByRole("button", { name: "Copy database path" }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${agenttik.dataDir}/agenttik.db`);
  await dialog.getByPlaceholder("Filter settings…").fill("database");
  await expect(path).toBeVisible();
  await expect(dialog.getByText("What the prompt box does", { exact: false })).toBeHidden();
});

test("reset defaults warns, cancels, and preserves browser data", async ({ page }) => {
  await openSettings(page, "Appearance");
  await expect(page.getByRole("button", { name: "blue", exact: true })).toHaveAttribute("aria-pressed", "true");
  await page.getByRole("button", { name: "red", exact: true }).click();
  await page.evaluate(() => localStorage.setItem("keep-auth", "signed-in"));
  await page.getByRole("button", { name: "General", exact: true }).click();
  await page.getByRole("button", { name: "Reset defaults", exact: true }).click();
  const warning = page.getByRole("dialog", { name: "Reset defaults?", exact: true });
  await expect(warning).toContainText("Accounts stay signed in");
  await warning.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(await page.evaluate(() => JSON.parse(localStorage.getItem("agenttik.colors")).accent)).toBe("red");
  await page.getByRole("button", { name: "Reset defaults", exact: true }).click();
  await warning.getByRole("button", { name: "Reset defaults", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Defaults restored" })).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem("keep-auth"))).toBe("signed-in");
  await page.getByRole("button", { name: "Appearance", exact: true }).click();
  await expect(page.getByRole("button", { name: "blue", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("button", { name: "Color mode", exact: true })).toContainText("System");
  await page.reload();
  await openSettings(page, "Appearance");
  await expect(page.getByRole("button", { name: "blue", exact: true })).toHaveAttribute("aria-pressed", "true");
});
