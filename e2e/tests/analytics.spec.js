import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

async function openAnalytics(page) {
  await page.keyboard.press("Control+Shift+p");
  await page.getByPlaceholder("Command Palette…").fill("Analytics");
  await page.getByText("Analytics", { exact: true }).click();
  return page.getByRole("dialog", { name: "Analytics", exact: true });
}

test("analytics opens from the palette and handles an empty period", async ({ page }) => {
  const dialog = await openAnalytics(page);
  await expect(dialog.getByText("No recorded usage in this period.")).toBeVisible();
  await dialog.getByRole("combobox", { name: "Analytics period" }).click();
  await page.getByRole("option", { name: "Last 24 hours", exact: true }).click();
  await expect(dialog.getByText("No recorded usage in this period.")).toBeVisible();
});

test("analytics ranks projects, expands tasks, separates subscriptions and retries", async ({ page }) => {
  const rows = [
    { provider: "fake", account_id: 0, project_id: 1, project_name: "Alpha", session_id: "a", title: "Alpha task", cost_usd: 2, input_tokens: 100, output_tokens: 20, turns: 1, cost_turns: 1, inferred_turns: 1 },
    { provider: "fake", account_id: 8, project_id: 2, project_name: "Beta", session_id: "b", title: "Beta task", cost_usd: 6, input_tokens: 50, output_tokens: 10, turns: 1, cost_turns: 1 },
  ];
  let fail = false;
  await page.route("**/api/analytics?*", async (route) => {
    if (fail) return route.fulfill({ status: 500, json: { error: "Analytics unavailable" } });
    const days = Number(new URL(route.request().url()).searchParams.get("days"));
    await route.fulfill({ json: { from: Date.now() - days * 86400000, to: Date.now(), rows } });
  });
  const dialog = await openAnalytics(page);
  await expect(dialog.getByText("$8.00", { exact: true })).toBeVisible();
  await expect(dialog.getByText(/older turns use/)).toBeVisible();
  const projectRows = dialog.locator(".analytics-table > tbody > tr:first-child");
  await expect(projectRows.first()).toContainText("Beta");
  await expect(projectRows.first()).toContainText("75.0%");
  await dialog.locator("summary").first().click();
  await expect(dialog.getByRole("button", { name: "Beta task" })).toBeVisible();
  await dialog.getByRole("combobox", { name: "Analytics ranking" }).click();
  await page.getByRole("option", { name: "Input tokens", exact: true }).click();
  await expect(projectRows.first()).toContainText("Alpha");
  await expect(projectRows.first()).toContainText("66.7%");
  await dialog.getByRole("combobox", { name: "Analytics grouping" }).click();
  await page.getByRole("option", { name: "Subscription / connection", exact: true }).click();
  await expect(dialog.locator("section")).toHaveCount(2);
  await expect(dialog.getByRole("heading", { name: /Removed subscription #8/ })).toBeVisible();
  fail = true;
  await dialog.getByRole("button", { name: "Refresh" }).click();
  await expect(dialog.getByRole("alert")).toContainText("Analytics unavailable");
  fail = false;
  await dialog.getByRole("button", { name: "Retry" }).click();
  await expect(dialog.getByText("$8.00", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(dialog.getByRole("combobox", { name: "Analytics period" })).toBeVisible();
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth)).toBe(true);
});


test("analytics includes persisted turn usage and opens its task", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "Analytics smoke test");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();
  const dialog = await openAnalytics(page);
  const project = dialog.locator(".analytics-table > tbody > tr:first-child");
  await expect(project).toContainText("$0.0042");
  await expect(project).toContainText("100.0%");
  await dialog.locator("summary").click();
  const task = dialog.getByRole("table", { name: "Task breakdown" }).getByRole("button");
  await expect(task).toBeVisible();
  await task.click();
  await expect(dialog).toBeHidden();
  await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();
});
