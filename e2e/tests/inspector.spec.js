import {
  addProject,
  expect,
  inspector,
  newTask,
  openProject,
  pickModel,
  sidebar,
  test,
} from "../fixtures.js";

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

test("a task's inspector keeps workspace panes and puts Stats in its header", async ({ page }) => {
  await newTask(page);

  await expect(inspector(page).getByRole("tab", { name: "Changed" })).toBeVisible();
  await expect(inspector(page).getByRole("tab", { name: "Commits" })).toBeVisible();
  await expect(inspector(page).getByRole("tab", { name: "Stats" })).toHaveCount(0);
  await expect(inspector(page).getByRole("tab", { name: "Options" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Task stats" })).toBeVisible();

  // A clean checkout has nothing changed, an edited one does; either is a
  // correct answer, so assert the pane rendered one of them.
  const changed = inspector(page)
    .getByText("No edited files.")
    .or(inspector(page).locator("button.font-mono").first());
  await expect(changed).toBeVisible();
});

test("the pane in use survives switching from a project to a task", async ({ page }) => {
  // Commits is the one pane both a project and a task offer.
  await inspector(page).getByRole("tab", { name: "Commits" }).click();

  await newTask(page);
  await pickModel(page);

  await expect(inspector(page).getByRole("tab", { name: "Options" })).toHaveCount(0);
  await expect(inspector(page).getByRole("tab", { name: "Commits" })).toHaveAttribute(
    "data-state",
    "active",
  );

  // The header popover belongs to the task, not its project: Permission is a
  // session row only task Stats shows.
  await page.getByRole("button", { name: "Task stats", exact: true }).click();
  await expect(page.getByLabel("Task statistics").getByText("Permission")).toBeVisible();
});

test("Command Palette opens the task Stats popover", async ({ page }) => {
  await newTask(page);

  await page.keyboard.press("Control+Shift+p");
  const goTo = page.getByRole("dialog", { name: "Command Palette" });
  await goTo.getByText("Task stats", { exact: true }).click();

  await expect(page.getByLabel("Task statistics")).toBeVisible();
});

test("commits offers fuzzy branch selection and branch actions", async ({ page }) => {
  let branch = "main";
  const requests = [];
  await page.route("**/api/projects/*/log?*", (route) => route.fulfill({
    json: { branch, branches: ["main", "feature/search", "fix/colors"], head: "12345678", commits: [] },
  }));
  await page.route("**/api/projects/*/branches/*", async (route) => {
    const action = new URL(route.request().url()).pathname.split("/").pop();
    const body = route.request().postDataJSON();
    requests.push({ action, ...body });
    if (action === "switch") branch = body.branch;
    await route.fulfill(action === "clean" ? { json: { deleted: ["old-feature"] } } : { status: 204 });
  });
  // Reload so the initial log request uses the fixture.
  await page.reload();
  await openProject(page);
  await inspector(page).getByRole("tab", { name: "Commits" }).click();
  const select = inspector(page).getByRole("button", { name: "Current branch", exact: true });
  await select.click();
  await page.getByPlaceholder("Search branches…").fill("ftsr");
  await expect(page.getByRole("option", { name: "feature/search" })).toBeVisible();
  await expect(page.getByRole("option", { name: "fix/colors" })).toHaveCount(0);
  await page.getByRole("option", { name: "feature/search" }).click();
  await expect(select).toContainText("feature/search");
  for (const name of ["Pull current branch", "Push current branch", "Clean local branches merged into main or master"]) {
    await inspector(page).getByRole("button", { name, exact: true }).click();
  }
  await expect(inspector(page).getByRole("status")).toContainText("Removed: old-feature");
  expect(requests).toEqual(["switch", "pull", "push", "clean"].map((action) => ({ action, branch: "feature/search" })));
});
