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
  await expect(inspector(page).getByRole("tab", { name: "Project Options" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Task stats" })).toBeVisible();

  // A clean checkout has nothing changed, an edited one does; either is a
  // correct answer, so assert the pane rendered one of them.
  const changed = inspector(page)
    .getByText("No edited files.")
    .or(inspector(page).locator("button.font-mono").first());
  await expect(changed).toBeVisible();
});

test("the pane in use survives switching from a project to a task", async ({ page }) => {
  // Shared panes keep their selection when switching to a task.
  await inspector(page).getByRole("tab", { name: "Commits" }).click();

  await newTask(page);
  await pickModel(page);

  await expect(inspector(page).getByRole("tab", { name: "Project Options" })).toHaveCount(0);
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

test("commit graph toggle, counts, checkout highlight and ref chips", async ({ page }) => {
  const commits = [
    { hash: "aaaaaaaa", subject: "Merge feature", author: "Test Author", date: "2026-09-15 12:00", parents: ["bbbbbbbb", "cccccccc"], branches: ["main"], tags: ["v1.0"], fileCount: 7 },
    { hash: "cccccccc", subject: "Feature work", author: "Test Author", date: "2026-09-15 11:00", parents: ["dddddddd"], branches: ["feature", "origin/feature"], tags: [], fileCount: 32 },
    { hash: "bbbbbbbb", subject: "Main work", author: "Test Author", date: "2026-09-15 10:00", parents: ["dddddddd"], branches: [], tags: [], fileCount: 1 },
    { hash: "dddddddd", subject: "Initial commit", author: "Test Author", date: "2026-09-15 09:00", parents: [], branches: [], tags: [], fileCount: 2 },
  ];
  await page.route("**/api/projects/*/log?*", (route) => route.fulfill({
    json: { branch: "main", branches: ["main", "feature"], head: "aaaaaaaa", commits },
  }));
  await page.route("**/api/projects/*/commit?*", (route) => route.fulfill({
    json: { hash: "aaaaaaaa", files: [{ path: "file.txt", status: "M", additions: 2, deletions: 1 }] },
  }));
  await page.route("**/api/projects/*/branches/clean?*", (route) => route.fulfill({ json: { deleted: [] } }));
  await page.reload();
  await openProject(page);
  const pane = inspector(page);
  await pane.getByRole("tab", { name: "Commits" }).click();
  const toggle = pane.getByRole("button", { name: "Show commit graph", exact: true });
  const head = pane.locator('[data-hash="aaaaaaaa"]');
  await expect(toggle).toHaveAttribute("aria-pressed", "false");
  await expect(head.locator("button[aria-current=true]")).toBeVisible();
  await expect(head.getByLabel("7 files affected")).toHaveText("[7]");
  await expect(pane.getByText("aaaaaaaa", { exact: true })).toHaveCount(0);
  await expect(head.getByText("v1.0", { exact: true })).toHaveCount(0);
  await expect(head.getByText("main", { exact: true })).toBeVisible();
  await expect(head.locator("span.font-mono")).toHaveText("aaaaaaaa · Test Author · 2026-09-15 12:00");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-pressed", "true");
  await expect(head.locator(":scope > svg")).toBeVisible();
  await expect(head.getByText("main", { exact: true })).toBeVisible();
  await expect(head.locator("span.font-mono")).toHaveText("aaaaaaaa · Test Author · 2026-09-15 12:00");
  await expect(head.getByText("v1.0", { exact: true })).toBeVisible();
  await expect(pane.getByText("origin/feature", { exact: true })).toBeVisible();
  await head.getByRole("button").click();
  await expect(head.getByRole("button", { name: /file.txt/ })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("commits-graph.png") });
  await pane.getByPlaceholder("Filter commits").fill("work");
  await expect(pane.locator("[data-hash]")).toHaveCount(2);
  await expect(pane.locator("[data-hash]").first()).toHaveAttribute("data-hash", "cccccccc");
  await pane.getByPlaceholder("Filter commits").clear();
  await toggle.click();
  await expect(head.locator(":scope > svg")).toHaveCount(0);
  await expect(head.getByText("v1.0", { exact: true })).toHaveCount(0);
  await expect(head.getByText("main", { exact: true })).toBeVisible();
  await expect(head.locator("span.font-mono")).toHaveText("aaaaaaaa · Test Author · 2026-09-15 12:00");
  await pane.getByRole("button", { name: "Clean local branches merged into main or master", exact: true }).click();
  await expect(pane.getByRole("status")).toHaveText("No merged local branches to remove.");
});

test("merge and rebase choose a destination and show recovery", async ({ page }) => {
  let branch = "feature/search";
  let operation = "";
  const requests = [];
  await page.route("**/api/projects/*/log?*", (route) => route.fulfill({
    json: { branch, operation, branches: ["main", "feature/search", "fix/colors"], head: "12345678", commits: [] },
  }));
  await page.route("**/api/projects/*/branches/*", async (route) => {
    const action = new URL(route.request().url()).pathname.split("/").pop();
    const body = route.request().postDataJSON();
    requests.push({ action, ...body });
    if (action === "merge") branch = body.target;
    if (action === "rebase") operation = "rebase";
    if (action === "abort") operation = "";
    await route.fulfill({ json: { message: action === "rebase" ? "Rebase paused." : "Completed." } });
  });
  await page.reload();
  await openProject(page);
  const pane = inspector(page);
  await pane.getByRole("tab", { name: "Commits" }).click();
  const merge = pane.getByRole("button", { name: "Merge & solve conflicts", exact: true });
  await expect(merge).toBeDisabled();
  await pane.getByRole("button", { name: "Destination branch", exact: true }).click();
  await expect(page.getByRole("option", { name: "feature/search", exact: true })).toHaveCount(0);
  await page.getByRole("option", { name: "fix/colors", exact: true }).click();
  await expect(pane.getByText("If there are conflicts, AI will solve them", { exact: false })).toBeVisible();
  await merge.click();
  await expect(pane.getByRole("button", { name: "Current branch", exact: true })).toContainText("fix/colors");
  await pane.getByRole("combobox", { name: "Branch operation", exact: true }).click();
  await page.getByRole("option", { name: "rebase into", exact: true }).click();
  await pane.getByRole("button", { name: "Destination branch", exact: true }).click();
  await page.getByRole("option", { name: "main", exact: true }).click();
  await pane.getByRole("button", { name: "Rebase & solve conflicts", exact: true }).click();
  await expect(pane.getByRole("button", { name: "Retry solving conflicts", exact: true })).toBeVisible();
  await expect(pane.getByRole("button", { name: "Current branch", exact: true })).toBeDisabled();
  await pane.getByRole("button", { name: "Abort", exact: true }).click();
  await expect(pane.getByRole("button", { name: "Rebase & solve conflicts", exact: true })).toBeVisible();
  expect(requests).toEqual([
    { action: "merge", branch: "feature/search", target: "fix/colors" },
    { action: "rebase", branch: "fix/colors", target: "main" },
    { action: "abort", branch: "fix/colors" },
  ]);
});
