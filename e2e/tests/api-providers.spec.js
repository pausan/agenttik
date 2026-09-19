import { addProject, expect, modelButton, newTask, openSettings, test } from "../fixtures.js";

test("settings tree groups sections, collapses, and searches descendants", async ({ page }, testInfo) => {
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  const nav = dialog.getByRole("list", { name: "Settings sections", exact: true });
  expect(await nav.locator(":scope > li").evaluateAll((rows) => rows.map((row) => row.querySelector(":scope > div > button:last-child").innerText.trim())))
    .toEqual(["General", "Orchestrator", "Providers", "Models", "Server", "Help", "About"]);
  await expect(nav.getByRole("list", { name: "General sections", exact: true }).getByRole("button"))
    .toHaveText(["Appearance", "Profiles", "Projects", "Shortcuts"]);
  await expect(nav.getByRole("list", { name: "Providers sections", exact: true }).getByRole("button"))
    .toHaveText(["Subscriptions", "API Providers"]);
  await dialog.screenshot({ path: testInfo.outputPath("settings-tree.png") });
  await nav.getByRole("button", { name: "Collapse Providers", exact: true }).click();
  await expect(nav.getByRole("button", { name: "API Providers", exact: true })).toBeHidden();
  await dialog.getByPlaceholder("Filter settings…").fill("OpenRouter");
  await expect(nav.getByRole("button", { name: /^API Providers/ })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Provider", exact: true })).toBeVisible();
  await dialog.getByPlaceholder("Filter settings…").fill("");
  await nav.getByRole("button", { name: "Models", exact: true }).click();
  await expect(dialog.getByRole("group", { name: "System · Fake models", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: /OpenCode.*models/ })).toHaveCount(0);
  await expect(dialog.getByRole("group", { name: /OpenAI.*models/ })).toHaveCount(0);
});

test("API keys enable models, select tasks, hide when disabled, and can be removed", async ({ page }) => {
  let providers;
  let savedKey = false;
  let enabled = false;
  const responses = () => providers.map((p) => {
    if (p.name === "fake") return { ...p, accounts: [...p.accounts, { id: 9876, alias: "Signed out", signed_in: false }] };
    if (p.name !== "api-openai") return p;
    return { ...p, available: enabled, api: { ...p.api, enabled, key_set: savedKey },
      models: enabled ? [{ id: "gpt-test", label: "API Test Model" }] : [],
      accounts: [{ id: 0, alias: "API", system: true, is_default: true, signed_in: enabled }] };
  });
  await page.route("**/api/providers", async (route) => {
    const response = await route.fetch();
    providers = await response.json();
    await route.fulfill({ response, json: responses() });
  });
  await page.route("**/api/providers/api-openai/api", async (route) => {
    if (route.request().method() === "DELETE") { savedKey = false; enabled = false; }
    else {
      const body = route.request().postDataJSON();
      if (body.key === "invalid-key") { await route.fulfill({ status: 400, json: { error: "Provider rejected the key" } }); return; }
      if (body.key) { expect(body.key).toBe("ui-test-only-key"); savedKey = true; }
      enabled = route.request().method() === "POST" || body.enabled;
    }
    await route.fulfill({ json: responses() });
  });
  // The Go integration suite exercises real key storage and session dispatch.
  // This browser fixture keeps every paid provider call mocked, including selection.
  await page.route("**/api/sessions/*", async (route) => {
    const request = route.request();
    if (request.method() !== "PATCH" || (request.postDataJSON()?.provider !== "api-openai" && !request.postDataJSON()?.permission)) return route.continue();
    const current = await page.request.get(request.url());
    const detail = await current.json();
    await route.fulfill({ json: { ...detail.session, provider: "api-openai", model: "gpt-test", ...request.postDataJSON() } });
  });
  await addProject(page);
  await openSettings(page, "API Providers");
  const dialog = page.getByRole("dialog");
  const provider = dialog.getByRole("group", { name: "OpenAI API", exact: true });
  const form = dialog.getByRole("form", { name: "Add API provider" });
  await expect(form.getByRole("button", { name: "Add", exact: true })).toBeDisabled();
  await form.getByRole("button", { name: "Provider", exact: true }).click();
  await page.getByPlaceholder("Search providers…").fill("opnai");
  await page.getByRole("option", { name: "OpenAI", exact: true }).click();
  await expect(page.getByRole("listbox")).toBeHidden();
  await form.getByLabel("Name", { exact: true }).fill("Work");
  const key = form.getByLabel("API key", { exact: true });
  await key.fill("invalid-key");
  await form.getByRole("button", { name: "Add", exact: true }).click();
  await expect(form.getByRole("alert")).toContainText("Provider rejected the key");
  await key.fill("ui-test-only-key");
  await form.getByRole("button", { name: "Add", exact: true }).click();
  await expect(key).toHaveValue("");
  await expect(provider.getByText("1 model", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Models", exact: true }).click();
  await expect(dialog.getByRole("group", { name: "OpenAI API models", exact: true })).toContainText("API Test Model");
  await expect(dialog.getByRole("group", { name: /Signed out.*models/ })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Done", exact: true }).click();
  await newTask(page);
  await modelButton(page).click();
  await page.getByPlaceholder("Search models…").fill("API Test Model");
  const selected = page.waitForRequest((r) => r.method() === "PATCH" && r.url().includes("/api/sessions/"));
  await page.getByRole("option", { name: /API Test Model/ }).click();
  expect((await selected).postDataJSON()).toMatchObject({ provider: "api-openai", model: "gpt-test", account_id: 0 });
  await expect(modelButton(page)).toContainText("OpenAI API · API Test Model");
  await page.getByRole("combobox", { name: "Tool access", exact: true }).click();
  const permissionChange = page.waitForRequest((r) => r.method() === "PATCH" && r.postDataJSON()?.permission === "full");
  await page.getByRole("option", { name: "Full access", exact: true }).click();
  expect((await permissionChange).postDataJSON()).toEqual({ permission: "full" });
  await expect(page.getByRole("combobox", { name: "Tool access", exact: true })).toContainText("Full access");
  await openSettings(page, "API Providers");
  await provider.getByRole("button", { name: "Disable", exact: true }).click();
  await expect(provider.getByText("Disabled", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Models", exact: true }).click();
  await expect(dialog.getByRole("group", { name: "OpenAI API models", exact: true })).toHaveCount(0);
  await dialog.getByRole("button", { name: "API Providers", exact: true }).click();
  await provider.getByRole("button", { name: "Enable", exact: true }).click();
  await expect(provider.getByText("1 model", { exact: true })).toBeVisible();
  await provider.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(provider).toHaveCount(0);
  expect(await page.evaluate(() => JSON.stringify(localStorage))).not.toContain("ui-test-only-key");
});

test("the settings tree and API setup fit a phone", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await page.getByRole("button", { name: "Projects and tasks", exact: true }).click();
  await page.getByRole("dialog", { name: "Projects and tasks", exact: true }).getByRole("button", { name: "Settings", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Settings", exact: true });
  await dialog.getByRole("button", { name: "API Providers", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "Provider", exact: true })).toBeVisible();
  await dialog.screenshot({ path: testInfo.outputPath("settings-api-phone.png") });
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBeTruthy();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
});

test("multiple named connections to one provider can be added and deleted independently", async ({ page }) => {
  let templates = [];
  let connections = [];
  let sequence = 0;
  await page.route("**/api/providers", async (route) => {
    const response = await route.fetch();
    templates = await response.json();
    await route.fulfill({ response, json: [...templates, ...connections] });
  });
  await page.route("**/api/providers/*/api", async (route) => {
    if (route.request().method() === "POST") {
      const body = route.request().postDataJSON();
      const template = templates.find((p) => p.name === "api-openrouter");
      connections.push({ ...template, name: `api-openrouter-${++sequence}`, display_name: `${body.name} · OpenRouter API`,
        available: true, models: [{ id: "test-model", label: "Test model" }],
        api: { ...template.api, catalog: false, enabled: true, key_set: true } });
    } else {
      const id = route.request().url().split("/").at(-2);
      connections = connections.filter((p) => p.name !== id);
    }
    await route.fulfill({ json: [...templates, ...connections] });
  });
  await openSettings(page, "API Providers");
  const dialog = page.getByRole("dialog");
  const form = dialog.getByRole("form", { name: "Add API provider" });
  await form.getByRole("button", { name: "Provider", exact: true }).click();
  await page.getByPlaceholder("Search providers…").fill("opnrt");
  await expect(page.getByRole("option")).toHaveCount(1);
  await page.getByRole("option", { name: "OpenRouter", exact: true }).click();
  await expect(page.getByRole("listbox")).toBeHidden();
  for (const name of ["Work", "Personal", "Work research"]) {
    await form.getByLabel("Name", { exact: true }).fill(name);
    await form.getByLabel("API key", { exact: true }).fill(`test-${name.replaceAll(" ", "-")}`);
    await form.getByRole("button", { name: "Add", exact: true }).click();
    await expect(dialog.getByRole("group", { name: `${name} · OpenRouter API`, exact: true })).toBeVisible();
    await expect(form.getByLabel("API key", { exact: true })).toHaveValue("");
  }
  const work = dialog.getByRole("group", { name: "Work · OpenRouter API", exact: true });
  await work.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(work).toHaveCount(0);
  await expect(dialog.getByRole("group", { name: "Personal · OpenRouter API", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "Work research · OpenRouter API", exact: true })).toBeVisible();
});
