import { expect, openSettings, test } from "../fixtures.js";

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
