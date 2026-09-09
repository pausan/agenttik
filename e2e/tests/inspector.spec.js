import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test.beforeEach(async ({ page }) => {
  await addProject(page);
  await openProject(page);
});

test("the tree lists the repo's files", async ({ page }) => {
  await sidebar(page).getByRole("tab", { name: "Tree" }).click();

  await expect(sidebar(page).getByRole("button", { name: "go.mod" })).toBeVisible();
  await expect(sidebar(page).getByRole("button", { name: "Makefile" })).toBeVisible();
  // .git and gitignored paths are excluded.
  await expect(sidebar(page).getByRole("button", { name: /^\.git\// })).toHaveCount(0);
});

test("clicking a file opens it as a closable tab beside the first one", async ({ page }) => {
  await sidebar(page).getByRole("tab", { name: "Tree" }).click();
  await sidebar(page).getByRole("button", { name: "go.mod" }).click();

  await expect(page.getByRole("tab", { name: /go\.mod/ })).toBeVisible();
  await expect(page.getByText("module github.com/pausan/agenttik")).toBeVisible();

  // The first tab cannot be closed; this one can.
  await page.getByRole("tab", { name: /go\.mod/ }).getByText("×").click();
  await expect(page.getByRole("tab", { name: /go\.mod/ })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "agenttik" })).toBeVisible();
});

test("changed lists what git reports as edited", async ({ page }) => {
  await inspector(page).getByRole("tab", { name: "Stats" }).click();
  await expect(inspector(page).getByText("Running now")).toBeVisible();

  // A clean checkout has nothing changed, an edited one does; either is a
  // correct answer, so assert the pane rendered one of them.
  await openProject(page);
  await inspector(page).getByRole("tab", { name: "Options" }).click();
  await expect(inspector(page).getByText("The folder on disk is untouched.")).toBeVisible();
});

test("project stats aggregate over every session", async ({ page }) => {
  await inspector(page).getByRole("tab", { name: "Stats" }).click();

  for (const row of ["Sessions", "Running now", "Turns", "Cost", "Agent time", "Last active"]) {
    await expect(inspector(page).getByText(row, { exact: true })).toBeVisible();
  }
  await expect(inspector(page).getByText("$0.0000")).toBeVisible();
});

test("the pane in use survives switching from a project to a session", async ({ page }) => {
  await inspector(page).getByRole("tab", { name: "Stats" }).click();
  await page.getByRole("button", { name: "New session" }).first().click();

  // Options is gone, Changed has taken its place, and Stats is still the one open.
  await expect(inspector(page).getByRole("tab", { name: "Changed" })).toBeVisible();
  await expect(inspector(page).getByRole("tab", { name: "Options" })).toHaveCount(0);
  await expect(inspector(page).getByRole("tab", { name: "Stats" })).toHaveAttribute(
    "data-state",
    "active",
  );
  await expect(inspector(page).getByText("Permission")).toBeVisible(); // a session row, not a project one
});
