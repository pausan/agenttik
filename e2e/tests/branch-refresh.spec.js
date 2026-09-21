import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, newTask, inspector, test, expect } from "../fixtures.js";

test("commits follows branch switches outside the UI without working-tree changes", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "branch-refresh-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q", "-b", "main");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    git("commit", "--allow-empty", "-qm", "Initial commit");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    const branch = pane.getByRole("button", { name: "Current branch", exact: true });
    await expect(branch).toContainText("main");

    git("switch", "-qc", "feature/task");
    await expect(branch).toContainText("feature/task");
    await branch.click();
    await expect(page.getByRole("option", { name: "feature/task", exact: true })).toBeVisible();
    await expect(page.getByRole("option", { name: "main", exact: true })).toBeVisible();
    await page.keyboard.press("Escape");

    git("commit", "--allow-empty", "-qm", "Task commit");
    await expect(pane.getByText("Task commit", { exact: true })).toBeVisible();
    git("switch", "-q", "main");
    await expect(branch).toContainText("main");
    await expect(pane.getByText("Task commit", { exact: true })).toBeHidden();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
