import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, newTask, inspector, test as base, expect } from "../fixtures.js";

const test = base.extend({
  repos: async ({}, use) => {
    const root = await mkdtemp(join(tmpdir(), "commit-drafts-"));
    const project = join(root, "project");
    const other = join(root, "other");
    try {
      for (const cwd of [join(project, "alpha"), join(project, "beta"), other]) {
        await mkdir(cwd, { recursive: true });
        execFileSync("git", ["init", "-q"], { cwd });
        await writeFile(join(cwd, "file.txt"), "new file\n");
        execFileSync("git", ["add", "."], { cwd });
      }
      await use({ project, other });
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  },
});

async function showChanges(page, project) {
  await openProject(page, project);
  await inspector(page).getByRole("tab", { name: "Changed", exact: true }).click();
  await expect(inspector(page).getByRole("button", { name: /^Staged/ })).toHaveText("Staged 1");
}

async function selectRepo(page, repo) {
  await inspector(page).getByRole("combobox", { name: "Git repository" }).click();
  await page.getByRole("option", { name: repo, exact: true }).click();
}

test("commit drafts survive pane, task and file tabs, reloads and manual clearing", async ({ page, repos }) => {
  await addProject(page, repos.project);
  await showChanges(page, repos.project);
  const pane = inspector(page);
  const field = pane.getByRole("textbox", { name: "Commit message" });
  const draft = "  Keep this half-written subject\n\nBody still in progress\n";
  await field.fill(draft);
  for (const tab of ["Commits", "Project Options"]) {
    await pane.getByRole("tab", { name: tab, exact: true }).click();
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    await expect(field).toHaveValue(draft);
  }
  await newTask(page);
  await expect(field).toHaveValue(draft);
  await pane.locator('button[data-path="alpha/file.txt"]').click();
  await expect(page.getByRole("tab", { name: /file.txt/ })).toBeVisible();
  await expect(field).toHaveValue(draft);
  await page.reload();
  await expect(field).toHaveValue(draft);
  await field.fill(" \n ");
  await page.reload();
  await expect(field).toHaveValue(" \n ");
  await field.clear();
  await page.reload();
  await expect(field).toHaveValue("");
});

test("projects and repositories retain separate commit drafts", async ({ page, repos }) => {
  await addProject(page, repos.project);
  await showChanges(page, repos.project);
  const field = inspector(page).getByRole("textbox", { name: "Commit message" });
  await field.fill("Alpha draft");
  await selectRepo(page, "beta");
  await expect(field).toHaveValue("");
  await field.fill("Beta draft");
  await selectRepo(page, "alpha");
  await expect(field).toHaveValue("Alpha draft");
  await addProject(page, repos.other);
  await showChanges(page, repos.other);
  await expect(field).toHaveValue("");
  await field.fill("Other project draft");
  await showChanges(page, repos.project);
  await expect(field).toHaveValue("Alpha draft");
  await selectRepo(page, "beta");
  await expect(field).toHaveValue("Beta draft");
  await page.reload();
  // Repository selection starts at alpha; both drafts remain saved.
  await expect(field).toHaveValue("Alpha draft");
  await selectRepo(page, "beta");
  await expect(field).toHaveValue("Beta draft");
  await showChanges(page, repos.other);
  await expect(field).toHaveValue("Other project draft");
});

for (const navigation of ["pane", "repository", "project"]) {
  test(`late generated messages preserve drafts after changing ${navigation}`, async ({ page, repos }) => {
    await addProject(page, repos.other);
    await addProject(page, repos.project);
    await showChanges(page, repos.project);
    const pane = inspector(page);
    const field = pane.getByRole("textbox", { name: "Commit message" });
    await field.fill("Original draft");
    let finish;
    const pending = new Promise((resolve) => { finish = resolve; });
    await page.route("**/api/projects/*/commit-message?*", async (route) => {
      await pending;
      await route.fulfill({ json: { message: "Late generated message" } });
    }, { times: 1 });
    try {
      const request = page.waitForRequest("**/api/projects/*/commit-message?*");
      await pane.getByRole("button", { name: "Generate commit message", exact: true }).click();
      await request;
      if (navigation === "pane") {
        await pane.getByRole("tab", { name: "Commits", exact: true }).click();
        await pane.getByRole("tab", { name: "Changed", exact: true }).click();
        await field.fill("New draft");
      } else if (navigation === "repository") {
        await selectRepo(page, "beta");
      } else {
        await showChanges(page, repos.other);
      }
      const response = page.waitForResponse("**/api/projects/*/commit-message?*");
      finish();
      await response;
      await expect(field).toBeEnabled();
      if (navigation !== "pane") await field.fill("New draft");
      await expect(field).toHaveValue("New draft");
      await showChanges(page, repos.project);
      if (navigation === "repository") await selectRepo(page, "alpha");
      await expect(field).toHaveValue(navigation === "pane" ? "New draft" : "Original draft");
      await page.reload();
      await expect(field).toHaveValue(navigation === "pane" ? "New draft" : "Original draft");
    } finally {
      finish();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}

for (const outcome of ["success", "failure", "edited"]) {
  test(`a commit finishing after navigation preserves the right drafts (${outcome})`, async ({ page, repos }) => {
    await addProject(page, repos.other);
    await addProject(page, repos.project);
    await showChanges(page, repos.project);
    const pane = inspector(page);
    const field = pane.getByRole("textbox", { name: "Commit message" });
    await field.fill("Submitted draft");
    let finish;
    const pending = new Promise((resolve) => { finish = resolve; });
    await page.route("**/api/projects/*/commit?*", async (route) => {
      expect(route.request().postDataJSON()).toEqual({ message: "Submitted draft" });
      await pending;
      await route.fulfill(outcome === "failure"
        ? { status: 400, json: { error: "Commit failed" } }
        : { status: 204 });
    }, { times: 1 });
    try {
      const request = page.waitForRequest("**/api/projects/*/commit?*");
      await pane.getByRole("button", { name: "Commit", exact: true }).click();
      await request;
      await pane.getByRole("tab", { name: "Commits", exact: true }).click();
      if (outcome === "edited") {
        await pane.getByRole("tab", { name: "Changed", exact: true }).click();
        await field.fill("Next commit draft");
      }
      await showChanges(page, repos.other);
      await field.fill("Other project draft");
      const response = page.waitForResponse("**/api/projects/*/commit?*");
      finish();
      await response;
      await expect(field).toHaveValue("Other project draft");
      await showChanges(page, repos.project);
      await expect(field).toHaveValue(outcome === "success" ? "" : outcome === "edited" ? "Next commit draft" : "Submitted draft");
      await page.reload();
      await expect(field).toHaveValue(outcome === "success" ? "" : outcome === "edited" ? "Next commit draft" : "Submitted draft");
    } finally {
      finish();
      await page.unrouteAll({ behavior: "wait" });
    }
  });
}
