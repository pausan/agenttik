import { REPO, addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test("opens with nothing selected", async ({ page }) => {
  await expect(page.getByText("No projects yet.")).toBeVisible();
  await expect(page.getByText("Pick a session, or a project to start one.")).toBeVisible();
});

test("a project appears in the sidebar with its folder underneath", async ({ page }) => {
  await addProject(page);
  await expect(sidebar(page).getByText("agenttik", { exact: true })).toHaveCount(1);
  await expect(sidebar(page).getByText(REPO)).toBeVisible();
});

test("opening a project puts its panes on the right and Tree on the left", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await expect(page.getByRole("tab", { name: "agenttik" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "agenttik" })).toBeVisible();
  await expect(page.getByText("Nothing open in this project.")).toBeVisible();

  for (const pane of ["Options", "Stats"]) {
    await expect(inspector(page).getByRole("tab", { name: pane })).toBeVisible();
  }
  await expect(sidebar(page).getByRole("tab", { name: "Tree" })).toBeVisible();
  // A project has no prompt bar: there is no conversation to prompt.
  await expect(page.getByPlaceholder("Ask the agent…")).toBeHidden();
});

test("a project is renamed from its options", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await inspector(page).getByRole("textbox").fill("renamed");
  await inspector(page).getByRole("textbox").blur();

  await expect(page.getByRole("tab", { name: "renamed" })).toBeVisible();
  await expect(sidebar(page).getByText("renamed")).toBeVisible();
});

test("deleting a project asks first, then clears the centre", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await inspector(page).getByRole("button", { name: "Delete project" }).click();
  await expect(page.getByText("Its sessions and their history are deleted")).toBeVisible();

  // Backing out leaves the project alone.
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(sidebar(page).getByText(REPO)).toBeVisible();

  await inspector(page).getByRole("button", { name: "Delete project" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete", exact: true }).click();

  await expect(page.getByText("No projects yet.")).toBeVisible();
  await expect(page.getByText("Pick a session, or a project to start one.")).toBeVisible();
});

test("the sessions tab filters across projects", async ({ page }) => {
  await addProject(page);
  await page.getByRole("tab", { name: "Sessions" }).click();

  await expect(page.getByText("No sessions in this window.")).toBeVisible();
  await expect(page.getByPlaceholder("Filter sessions")).toBeVisible();
  await expect(page.getByRole("combobox")).toContainText("Last 3 days");
});
