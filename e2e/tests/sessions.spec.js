import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test("a new session asks nothing and opens ready to prompt", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await page.getByRole("button", { name: "New session" }).first().click();

  await expect(page.getByRole("tab", { name: "Conversation" })).toBeVisible();
  await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();
  await expect(page.getByRole("button", { name: "Send" })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Stop" })).toBeHidden();
  await expect(page.getByText("idle")).toBeVisible();

  // It shows up in both sidebar tabs.
  await expect(sidebar(page).getByText("Untitled session")).toBeVisible();
  await page.getByRole("tab", { name: "Sessions" }).click();
  await expect(sidebar(page).getByText("Untitled session")).toBeVisible();
});

test("an empty prompt sends nothing", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await page.getByRole("button", { name: "New session" }).first().click();

  await page.getByPlaceholder("Ask the agent…").fill("   ");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("running…")).toBeHidden();
  await expect(page.getByText("You", { exact: true })).toHaveCount(0);
});

test("setup lists the providers and whether their CLI is installed", async ({ page }) => {
  await page.getByRole("button", { name: "Setup" }).click();

  await expect(page.getByText("Claude Code", { exact: true })).toBeVisible();
  await expect(page.getByText("Codex", { exact: true })).toBeVisible();
  // Every model gets a chip per effort, plus default.
  await expect(page.getByRole("button", { name: "default", exact: true }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "xhigh", exact: true }).first()).toBeVisible();
});

test("a starred model and effort heads the picker and sets both at once", async ({ page }) => {
  await page.getByRole("button", { name: "Setup" }).click();
  const chip = page.getByRole("button", { name: "xhigh", exact: true }).first();
  await chip.click();
  await expect(page.getByRole("button", { name: "★ xhigh", exact: true }).first()).toBeVisible();
  await page.getByRole("button", { name: "Done" }).click();

  await addProject(page);
  await openProject(page);
  await page.getByRole("button", { name: "New session" }).first().click();

  const models = page.getByRole("combobox").first();
  await models.click();
  await expect(page.getByRole("option").first()).toContainText("★");
  await page.getByRole("option").first().click();

  // Picking the combination set the effort too.
  await expect(page.getByRole("combobox").nth(1)).toContainText("xhigh");
  await expect(page.getByText("xhigh")).not.toHaveCount(0);
});

test("changing the model updates the badge and the stats", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await page.getByRole("button", { name: "New session" }).first().click();

  await page.getByRole("combobox").first().click();
  await page.getByRole("option", { name: "Haiku" }).click();

  await expect(page.getByText("haiku", { exact: true })).toBeVisible();
  await inspector(page).getByRole("tab", { name: "Stats" }).click();
  await expect(inspector(page).getByText("haiku")).toBeVisible();
});

test("the transcript follows the session, and the prompt bar goes away with it", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await page.getByRole("button", { name: "New session" }).first().click();
  await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();

  await openProject(page);
  await expect(page.getByPlaceholder("Ask the agent…")).toBeHidden();
  await expect(page.getByText("1 session", { exact: true })).toBeVisible();
});
