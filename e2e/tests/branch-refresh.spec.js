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

for (const [action, refresh, failed] of [["merge", "tree", false], ["rebase", "log", false], ["merge", "changes", true]]) {
  test(`${action} stops spinning after ${failed ? "failure" : "completion"} while ${refresh} refresh waits`, async ({ page }) => {
    const root = await mkdtemp(join(tmpdir(), "branch-completion-"));
    const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
    let releaseOperation, releaseRefresh;
    const operationGate = new Promise((resolve) => { releaseOperation = resolve; });
    const refreshGate = new Promise((resolve) => { releaseRefresh = resolve; });
    let refreshing = false;
    let refreshRequests = 0;
    try {
      git("init", "-q", "-b", "main");
      git("config", "user.name", "Test");
      git("config", "user.email", "test@example.com");
      git("commit", "--allow-empty", "-qm", "Initial commit");
      git("switch", "-qc", "feature");
      git("commit", "--allow-empty", "-qm", "Feature commit");
      await addProject(page, root);
      await openProject(page, root);
      const pane = inspector(page);
      await pane.getByRole("tab", { name: "Commits" }).click();
      await expect(pane.getByRole("button", { name: "Current branch", exact: true })).toContainText("feature");
      await pane.getByText("Merge / rebase", { exact: true }).click();
      if (action === "rebase") {
        await pane.getByRole("combobox", { name: "Branch operation", exact: true }).click();
        await page.getByRole("option", { name: "rebase into", exact: true }).click();
      }
      await pane.getByRole("button", { name: "Destination branch", exact: true }).click();
      await page.getByRole("option", { name: "main", exact: true }).click();

      await page.route(`**/api/projects/*/${refresh}*`, async (route) => {
        if (refreshing) {
          refreshRequests++;
          await refreshGate;
          if (failed) {
            await route.fulfill({ status: 503, json: { error: "Refresh unavailable" } });
            return;
          }
        }
        await route.continue();
      });
      await page.route(`**/api/projects/*/branches/${action}?*`, async (route) => {
        const response = failed ? null : await route.fetch();
        await operationGate;
        refreshing = true;
        if (failed) await route.fulfill({ status: 400, json: { error: "Merge paused: test failure" } });
        else await route.fulfill({ response });
      });

      const button = pane.getByRole("button", { name: `${action === "merge" ? "Merge" : "Rebase"} & solve conflicts`, exact: true });
      await button.click();
      await expect(button.locator(".animate-spin")).toBeVisible();
      await expect(button).toBeDisabled();
      releaseOperation();
      await expect(pane.getByText(failed ? "Merge paused: test failure" : `${action === "merge" ? "Merge" : "Rebase"} completed.`, { exact: true })).toBeVisible();
      await expect.poll(() => refreshRequests).toBeGreaterThan(0);
      // The operation has answered, but none of the held refreshes have.
      await expect(button.locator(".animate-spin")).toHaveCount(0);
      await expect(pane.getByRole("combobox", { name: "Branch operation", exact: true })).toBeEnabled();
      releaseRefresh();
      if (!failed) {
        await expect(pane.getByRole("button", { name: "Current branch", exact: true })).toContainText(action === "merge" ? "main" : "feature");
        git("merge-base", "--is-ancestor", action === "merge" ? "feature" : "main", action === "merge" ? "main" : "feature");
      }
    } finally {
      releaseOperation();
      releaseRefresh();
      await page.unrouteAll({ behavior: "wait" });
      await rm(root, { recursive: true, force: true });
    }
  });
}
