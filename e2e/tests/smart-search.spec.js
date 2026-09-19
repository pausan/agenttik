import { addProject, expect, openProject, openSettings, test } from "../fixtures.js";

test("project task time filter combines with fuzzy search", async ({ page }) => {
  const now = Date.now();
  const tasks = [
    { id: "recent", title: "Recent task", last_active_at: now },
    { id: "week", title: "Weekly task", last_active_at: now - 2 * 86400000 },
    { id: "month", title: "Monthly task", last_active_at: now - 15 * 86400000 },
    { id: "old", title: "Old task", last_active_at: now - 40 * 86400000 },
  ].map((t) => ({ ...t, status: "idle", model: "fake", done_at: 0 }));
  await page.route("**/api/sessions?*project_id=**", (route) => route.fulfill({
    json: route.request().url().includes("only_done=true") ? [] : tasks,
  }));
  await addProject(page);
  await openProject(page);
  const main = page.locator("main");
  for (const [label, count] of [["Last 24h", 1], ["Last week", 2], ["Last month", 3], ["All times", 4]]) {
    await main.getByRole("combobox", { name: "Task time filter" }).click();
    await page.getByRole("option", { name: label, exact: true }).click();
    await expect(main.getByText(`${count}/4`, { exact: true })).toBeVisible();
  }
  await main.getByPlaceholder("Filter tasks").fill("wkly");
  await expect(main.getByText("Weekly task", { exact: true })).toBeVisible();
  await expect(main.getByText("Recent task", { exact: true })).toBeHidden();
});

test("Smart Search can be cancelled during download and stays disabled after reload", async ({ page }) => {
  await page.route("**/api/smart-search**", (route) => route.fulfill({
    json: { phase: "download", percent: 35, total: 0, version: 0 },
  }));
  await openSettings(page);
  const dialog = page.getByRole("dialog");
  const fuzzy = dialog.getByRole("radio", { name: "Fuzzy Search (default)", exact: true });
  await expect(fuzzy).toBeChecked();
  await dialog.getByRole("radio", { name: "Smart Search", exact: true }).click();
  await expect(dialog.getByRole("progressbar", { name: "Model download" })).toBeVisible();
  await fuzzy.click();
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  await page.reload();
  await openSettings(page);
  await expect(page.getByRole("radio", { name: "Fuzzy Search (default)", exact: true })).toBeChecked();
});

// Opt-in integration test: exercises the real ONNX model, not simulated vectors.
test("Bekko downloads, indexes and retrieves a task across languages", async ({ page, agenttik }) => {
  test.skip(!process.env.AGENTTIK_TEST_BEKKO, "Downloads the real Bekko model");
  test.setTimeout(240000);
  const browserAssets = [];
  page.on("request", (request) => browserAssets.push(request.url()));
  await page.context().route("https://huggingface.co/**", (route) => route.abort());
  await addProject(page);
  const projects = await (await page.request.get(agenttik.url + "/api/projects")).json();
  for (const title of ["Repair user login and authentication", "Change the background color"]) {
    const response = await page.request.post(agenttik.url + "/api/sessions", {
      data: { project_id: projects[0].id, provider: "fake", model: "fake", title },
    });
    expect(response.ok()).toBeTruthy();
  }
  await openSettings(page);
  await page.getByRole("radio", { name: "Smart Search", exact: true }).click();
  const status = page.getByRole("dialog").getByRole("status");
  await expect(status).toContainText(/Smart Search (ready|unavailable)/, { timeout: 180000 });
  await expect(status).toContainText("Smart Search ready · 2 tasks indexed");
  expect(browserAssets.some((url) => /onnx|\.wasm|smart-search\.worker/.test(url))).toBeFalsy();
  await page.keyboard.press("Escape");
  await openProject(page);
  await page.getByPlaceholder("Smart Search tasks").fill("Repair user login and authentication");
  await expect(page.locator("main").getByText("Repair user login and authentication", { exact: true })).toBeVisible({ timeout: 30000 });
  await page.getByPlaceholder("Smart Search tasks").fill("autenticación de usuarios");
  await expect(page.locator("main").getByText("Repair user login and authentication", { exact: true })).toBeVisible({ timeout: 30000 });
  await expect(page.locator("main").getByText("Change the background color", { exact: true })).toBeHidden();
  // Reload reuses the shared backend index; the browser never downloads model files.
  await page.reload();
  await expect(page.getByText(/Smart Search ready · 2 tasks indexed/)).toBeVisible({ timeout: 60000 });
  await page.getByPlaceholder("Smart Search tasks").fill("users cannot sign in");
  await expect(page.locator("main").getByText("Repair user login and authentication", { exact: true })).toBeVisible({ timeout: 30000 });
  const created = await page.request.post(agenttik.url + "/api/sessions", {
    data: { project_id: projects[0].id, provider: "fake", model: "fake", title: "users cannot sign in" },
  });
  expect(created.ok()).toBeTruthy();
  const task = await created.json();
  await expect(page.locator("main").getByText("users cannot sign in", { exact: true })).toBeVisible({ timeout: 30000 });
  await expect(page.getByText(/Smart Search ready · 3 tasks indexed/)).toBeVisible();
  expect((await page.request.delete(agenttik.url + "/api/sessions/" + task.id)).ok()).toBeTruthy();
  await expect(page.locator("main").getByText("users cannot sign in", { exact: true })).toBeHidden();
  await expect(page.getByText(/Smart Search ready · 2 tasks indexed/)).toBeVisible({ timeout: 30000 });
});
