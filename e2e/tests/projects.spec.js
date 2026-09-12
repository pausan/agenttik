import {
  REPO,
  addProject,
  expect,
  inspector,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  sidebar,
  test,
} from "../fixtures.js";

test("opens with nothing selected", async ({ page }) => {
  await expect(inspector(page).getByRole("heading", { name: "Workspace" })).toBeVisible();
  await expect(page.getByText("No projects yet.")).toBeVisible();
  await expect(page.getByText("Pick a task, or a project to start one.")).toBeVisible();
});

test("a project appears in the sidebar with its folder underneath", async ({ page }) => {
  await addProject(page);
  await expect(sidebar(page).getByText("agenttik", { exact: true })).toHaveCount(1);
  await expect(sidebar(page).getByText(REPO)).toBeVisible();
  await expect(sidebar(page).getByRole("combobox", { name: "Git repository" })).toHaveCount(0);
});

test("multiple projects do not show a repository selector", async ({ page }) => {
  await addProject(page);
  await addProject(page, REPO + "/web");
  await openProject(page, REPO + "/web");
  await expect(page.getByRole("combobox", { name: "Git repository" })).toHaveCount(0);
  await expect(sidebar(page).getByRole("heading", { name: "Workspace" })).toHaveCount(0);
});

test("opening a project puts its panes on the right and Tree on the left", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await expect(page.getByRole("tab", { name: "agenttik" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "agenttik" })).toBeVisible();
  await expect(page.getByText("No tasks in this project yet.")).toBeVisible();

  for (const pane of ["Options", "Commits"]) {
    await expect(inspector(page).getByRole("tab", { name: pane })).toBeVisible();
  }
  await expect(inspector(page).getByText("The folder on disk is untouched.")).toBeVisible();
  await expect(sidebar(page).getByRole("tab", { name: "Tree" })).toBeVisible();
  // A project has no prompt bar: there is no conversation to prompt.
  await expect(page.getByPlaceholder("Ask the agent…")).toBeHidden();
});

test("a project is renamed from its options", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  const name = inspector(page).getByRole("textbox", { name: "Name" });
  await name.fill("renamed");
  await name.blur();

  await expect(page.getByRole("tab", { name: "renamed" })).toBeVisible();
  await expect(sidebar(page).getByText("renamed")).toBeVisible();
});

test("deleting a project asks first, then clears the centre", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await inspector(page).getByRole("button", { name: "Delete project" }).click();
  await expect(page.getByText("Its tasks and their history are deleted")).toBeVisible();

  // Backing out leaves the project alone.
  await page.getByRole("button", { name: "Cancel" }).click();
  await expect(sidebar(page).getByText(REPO)).toBeVisible();

  await inspector(page).getByRole("button", { name: "Delete project" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Delete", exact: true }).click();

  await expect(page.getByText("No projects yet.")).toBeVisible();
  await expect(page.getByText("Pick a task, or a project to start one.")).toBeVisible();
});

test("the project's task list is filtered by title", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "alpha task");
  await expect(sidebar(page).getByText("alpha task")).toBeVisible();

  await openProject(page);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "bravo task");
  await expect(sidebar(page).getByText("bravo task")).toBeVisible();

  await openProject(page);
  await page.getByPlaceholder("Filter tasks").fill("alpha");
  // Scoped to the project page, not the whole document: the sidebar lists
  // every task regardless of this filter, and would otherwise still show the
  // one just excluded. Not exact, either: a title still refines in the
  // background (specs/020-task-titles.md) to a reply that keeps naming its
  // prompt, e.g. "Refined: alpha task", so a substring match is what stays
  // true whichever title is showing.
  const tasks = page.locator("main");
  await expect(tasks.getByText("alpha task")).toBeVisible();
  await expect(tasks.getByText("bravo task")).toHaveCount(0);
});

test("archiving a finished task writes what it came to under its row", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await pickModel(page);
  // The directive keeps the reply distinct from the prompt echoed above it,
  // so the assertions below match one bubble rather than two.
  await sendPrompt(page, "@wait 10\nthe scheduler is deterministic now");
  await expect(page.getByText("the scheduler is deterministic now", { exact: true })).toBeVisible();
  await expect(page.getByText("idle")).toBeVisible();

  await openProject(page);
  const tasks = page.locator("main");
  await tasks.getByTitle("Archive task").click();

  // The row goes grey at once and grows its outcome a beat later: the fake
  // provider answers the isolated summary request with "Outcome: " plus the
  // reply it was handed. See specs/051-task-outcomes.md.
  await expect(tasks.getByTitle("Unarchive task")).toBeVisible();
  await expect(tasks.getByText("Outcome: the scheduler is deterministic now")).toBeVisible();

  // Restoring keeps the summary out of the way: it describes a finished task,
  // and this one is back in the working list.
  await tasks.getByTitle("Unarchive task").click();
  await expect(tasks.getByText("Outcome: the scheduler is deterministic now")).toHaveCount(0);
});

test("project stats aggregate over every task", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await page.getByRole("tab", { name: "Stats" }).click();
  // Each row is a <dt>; "Tasks" is also the sibling tab's own label, so this
  // is scoped to the <dt> tag itself rather than matched as plain text.
  for (const row of ["Tasks", "Running now", "Turns", "Cost", "Agent time", "Last used"]) {
    await expect(page.locator("dt").filter({ hasText: row })).toBeVisible();
  }
  await expect(page.getByText("$0.0000")).toBeVisible();
});
