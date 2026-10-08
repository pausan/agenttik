import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, inspector, test, expect } from "../fixtures.js";

test("push offers origin, cancel and dismiss keep the branch unpublished", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "branch-push-"));
  const remote = await mkdtemp(join(tmpdir(), "branch-origin-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
  const remoteGit = (...args) => execFileSync("git", args, { cwd: remote, encoding: "utf8" }).trim();
  try {
    remoteGit("init", "--bare", "-q");
    git("init", "-q", "-b", "feature/topic");
    git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-qm", "Initial commit");
    git("remote", "add", "origin", remote);
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    const push = pane.getByRole("button", { name: "Push current branch", exact: true });
    const dialog = page.getByRole("dialog", { name: "Push branch to origin?", exact: true });
    for (const cancel of [true, false]) {
      await push.click();
      await expect(dialog).toBeVisible();
      await expect(dialog).toContainText("feature/topic");
      expect(remoteGit("for-each-ref", "--format=%(refname)", "refs/heads/")).toBe("");
      if (cancel) await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
      else await page.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      expect(remoteGit("for-each-ref", "--format=%(refname)", "refs/heads/")).toBe("");
    }
    await push.click();
    await dialog.getByRole("button", { name: "Push to origin", exact: true }).click();
    await expect(dialog).toBeHidden();
    expect(remoteGit("rev-parse", "refs/heads/feature/topic")).toBe(git("rev-parse", "HEAD"));
    expect(git("rev-parse", "--abbrev-ref", "@{upstream}")).toBe("origin/feature/topic");
    await push.click();
    await expect(push).toBeEnabled();
    await expect(dialog).toBeHidden();
    // A deleted remote branch must ask again despite the local upstream ref.
    remoteGit("update-ref", "-d", "refs/heads/feature/topic");
    await push.click();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(remote, { recursive: true, force: true });
  }
});

test("branches show local first, group remotes, and sync a selected remote locally", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "branch-picker-"));
  const remote = await mkdtemp(join(tmpdir(), "branch-picker-remote-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
  try {
    execFileSync("git", ["init", "--bare", "-q", remote]);
    git("init", "-q", "-b", "main");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    git("commit", "--allow-empty", "-qm", "Initial commit");
    git("remote", "add", "origin", remote);
    git("remote", "add", "other", remote);
    git("push", "origin", "HEAD:feature/topic");
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    const branch = pane.getByRole("button", { name: "Current branch", exact: true });
    const fetch = pane.getByRole("button", { name: "Fetch remote branches", exact: true });
    await fetch.click();
    await expect(fetch).toBeEnabled();
    await branch.click();
    const list = page.getByRole("listbox");
    await expect(list.getByText("Local", { exact: true })).toBeVisible();
    await expect(list.getByText("origin", { exact: true })).toBeVisible();
    await expect(list.getByText("other", { exact: true })).toBeVisible();
    await expect(list.getByRole("option")).toHaveText(["main", "origin/feature/topic", "other/feature/topic"]);
    await page.getByPlaceholder("Search branches…").fill("origin/topic");
    await expect(list.getByRole("option")).toHaveText(["origin/feature/topic"]);
    await list.getByRole("option", { name: "origin/feature/topic", exact: true }).click();
    await expect(branch).toContainText("feature/topic");
    expect(git("rev-parse", "--abbrev-ref", "@{upstream}")).toBe("origin/feature/topic");
    git("switch", "-q", "main");
    git("commit", "--allow-empty", "-qm", "Remote advance");
    const tip = git("rev-parse", "HEAD");
    git("push", "origin", "HEAD:feature/topic");
    await expect(branch).toContainText("main");
    await branch.click();
    await page.getByRole("option", { name: "origin/feature/topic", exact: true }).click();
    await expect(branch).toContainText("feature/topic");
    await expect(pane.getByText("Remote advance", { exact: true })).toBeVisible();
    expect(git("rev-parse", "HEAD")).toBe(tip);
    await branch.click();
    await page.getByRole("option", { name: "main", exact: true }).click();
    await expect(branch).toContainText("main");
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(remote, { recursive: true, force: true });
  }
});
