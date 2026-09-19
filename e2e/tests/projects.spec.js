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

test("a project can be hidden and fuzzy-restored from the hidden menu", async ({ page }) => {
  await addProject(page, REPO + "/app");
  await addProject(page, REPO + "/web");
  await addProject(page, REPO + "/e2e");

  const row = sidebar(page).locator(".sidebar-project").filter({ hasText: REPO + "/web" });
  const title = row.locator("button").nth(1);
  const hide = row.getByRole("button", { name: "Hide project" });
  const titleBox = await title.boundingBox();
  const hideBox = await hide.boundingBox();
  expect(hideBox.x).toBeGreaterThan(titleBox.x);
  await hide.click();
  await expect(sidebar(page).getByText(REPO + "/web", { exact: true })).toHaveCount(0);

  await sidebar(page).getByRole("button", { name: "Hidden projects" }).click();
  const search = page.getByPlaceholder("Find hidden projects…");
  await expect(search).toBeVisible();
  await expect(page.getByRole("option", { name: "web", exact: true })).toBeVisible();
  // The restore menu contains names only, not project paths.
  await expect(page.getByText(REPO + "/web", { exact: true })).toHaveCount(0);

  await search.fill("wb");
  const match = page.getByRole("option", { name: "web", exact: true });
  await expect(match).toBeVisible();
  await match.click();

  await expect(search).toBeVisible();
  await expect(match).toHaveCount(0);
  await sidebar(page).getByRole("button", { name: "Hidden projects" }).click();
  await expect(search).toBeHidden();
  await expect(sidebar(page).getByText(REPO + "/web", { exact: true })).toBeVisible();
  const rows = sidebar(page).locator(".sidebar-project");
  await expect(rows.nth(0)).toContainText(REPO + "/e2e");
  await expect(rows.nth(1)).toContainText(REPO + "/web");
  await expect(rows.nth(2)).toContainText(REPO + "/app");
  await page.reload();
  const reloadedRows = sidebar(page).locator(".sidebar-project");
  await expect(reloadedRows.nth(0)).toContainText(REPO + "/e2e");
  await expect(reloadedRows.nth(1)).toContainText(REPO + "/web");
  await expect(reloadedRows.nth(2)).toContainText(REPO + "/app");
});

test("hidden projects can be restored repeatedly and all at once while searching", async ({ page }) => {
  for (const folder of ["app", "web", "e2e"]) {
    await addProject(page, REPO + "/" + folder);
    await sidebar(page).locator(".sidebar-project").filter({ hasText: REPO + "/" + folder })
      .getByRole("button", { name: "Hide project" }).click();
    await expect(sidebar(page).getByText(REPO + "/" + folder, { exact: true })).toHaveCount(0);
  }

  const toggle = sidebar(page).getByRole("button", { name: "Hidden projects" });
  const search = page.getByPlaceholder("Find hidden projects…");
  await toggle.click();
  for (const folder of ["app", "web"]) {
    await page.getByRole("option", { name: folder, exact: true }).click();
    await expect(search).toBeVisible();
    await expect(page.getByRole("option", { name: folder, exact: true })).toHaveCount(0);
    await expect(sidebar(page).getByText(REPO + "/" + folder, { exact: true })).toBeVisible();
  }
  await toggle.click();
  await expect(search).toBeHidden();
  await sidebar(page).locator(".sidebar-project").filter({ hasText: REPO + "/web" })
    .getByRole("button", { name: "Hide project" }).click();
  await toggle.click();
  await search.fill("wb");
  await expect(page.getByRole("option", { name: "web", exact: true })).toBeVisible();
  await expect(page.getByRole("option", { name: "e2e", exact: true })).toHaveCount(0);
  const restoreAll = page.getByRole("button", { name: "Restore all", exact: true });
  await restoreAll.click();
  for (const folder of ["app", "web", "e2e"]) {
    await expect(sidebar(page).getByText(REPO + "/" + folder, { exact: true })).toBeVisible();
  }
  await expect(search).toBeVisible();
  await expect(restoreAll).toBeDisabled();
  await search.fill("");
  await expect(page.getByText("No hidden projects.", { exact: true })).toBeVisible();
  await toggle.click();
  await expect(search).toBeHidden();
});

test("swapping projects inverts visibility regardless of search and persists", async ({ page }) => {
  for (const folder of ["app", "web", "e2e"]) {
    await addProject(page, REPO + "/" + folder);
  }
  await sidebar(page).locator(".sidebar-project").filter({ hasText: REPO + "/web" })
    .getByRole("button", { name: "Hide project" }).click();
  await expect(sidebar(page).getByText(REPO + "/web", { exact: true })).toHaveCount(0);
  await sidebar(page).getByRole("button", { name: "Hidden projects" }).click();
  const search = page.getByPlaceholder("Find hidden projects…");
  await search.fill("no match");
  const swap = page.getByRole("button", { name: "Swap", exact: true });
  await swap.click();
  await expect(swap).toBeEnabled();
  await expect(search).toBeVisible();
  await expect(sidebar(page).getByText(REPO + "/web", { exact: true })).toBeVisible();
  for (const folder of ["app", "e2e"]) {
    await expect(sidebar(page).getByText(REPO + "/" + folder, { exact: true })).toHaveCount(0);
  }
  await search.fill("");
  for (const folder of ["app", "e2e"]) {
    await expect(page.getByRole("option", { name: folder, exact: true })).toBeVisible();
  }
  await page.reload();
  await expect(sidebar(page).locator(".sidebar-project")).toHaveCount(1);
  await expect(sidebar(page).getByText(REPO + "/web", { exact: true })).toBeVisible();
  await sidebar(page).getByRole("button", { name: "Hidden projects" }).click();
  await swap.click();
  await expect(swap).toBeEnabled();
  for (const folder of ["app", "e2e"]) {
    await expect(sidebar(page).getByText(REPO + "/" + folder, { exact: true })).toBeVisible();
  }
  await expect(page.getByRole("option", { name: "web", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Restore all", exact: true }).click();
  await expect(swap).toBeEnabled();
  await swap.click();
  await expect(swap).toBeEnabled();
  await expect(sidebar(page).locator(".sidebar-project")).toHaveCount(0);
  await swap.click();
  await expect(swap).toBeEnabled();
  await expect(sidebar(page).locator(".sidebar-project")).toHaveCount(3);
});

test("opening a project puts its panes on the right and Tree on the left", async ({ page }) => {
  await addProject(page);
  await openProject(page);

  await expect(page.getByRole("tab", { name: "agenttik" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "agenttik" })).toBeVisible();
  await expect(page.getByText("No tasks in this project yet.")).toBeVisible();

  for (const pane of ["Project Options", "Changed", "Commits"]) {
    await expect(inspector(page).getByRole("tab", { name: pane })).toBeVisible();
  }
  await expect(inspector(page).getByText("The folder on disk is untouched.")).toBeVisible();
  await inspector(page).getByRole("tab", { name: "Changed", exact: true }).click();
  await expect(inspector(page).getByText("No edited files.")
    .or(inspector(page).locator("button.font-mono").first())).toBeVisible();
  await inspector(page).getByRole("tab", { name: "Commits", exact: true }).click();
  await expect(inspector(page).getByPlaceholder("Filter commits")).toBeVisible();
  await inspector(page).getByRole("tab", { name: "Project Options", exact: true }).click();
  await expect(inspector(page).getByRole("textbox", { name: "Name", exact: true })).toBeVisible();
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

  await tasks.getByRole("tab", { name: "Archived", exact: true }).click();

  // The archive row grows its outcome a beat later: the fake
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
