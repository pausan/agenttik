import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, inspector, test, expect } from "../fixtures.js";

test("tag a commit and push tags and branches from the commit menu", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "commit-refs-"));
  const remote = await mkdtemp(join(tmpdir(), "commit-refs-remote-"));
  const git = (cwd, ...args) => execFileSync("git", args, { cwd, encoding: "utf8" }).trim();
  try {
    git(remote, "init", "-q", "--bare");
    git(root, "init", "-q", "-b", "main");
    git(root, "config", "user.name", "Test");
    git(root, "config", "user.email", "test@example.com");
    git(root, "commit", "-q", "--allow-empty", "-m", "First commit");
    const first = git(root, "rev-parse", "--short=8", "HEAD");
    git(root, "commit", "-q", "--allow-empty", "-m", "Second commit");
    git(root, "branch", "feature/old", first);
    git(root, "remote", "add", "origin", remote);
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    const row = pane.locator(`[data-hash="${first}"]`);

    await row.getByRole("button", { name: /First commit/ }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Add tag…", exact: true }).click();
    await page.getByRole("textbox", { name: "Tag name" }).fill("v1.0");
    await page.getByRole("textbox", { name: "Tag name" }).press("Enter");
    // Tags show in the simple view, not just the graph.
    await expect(row.getByText("v1.0", { exact: true })).toBeVisible();
    expect(git(root, "rev-parse", "--short=8", "v1.0^{commit}")).toBe(first);

    await row.getByRole("button", { name: /First commit/ }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Push tag v1.0 to origin", exact: true }).click();
    await expect(pane.getByRole("status")).toContainText("Pushed tag v1.0 to origin.");
    expect(git(remote, "rev-parse", "--short=8", "v1.0")).toBe(first);

    await row.getByRole("button", { name: /First commit/ }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Push branch feature/old to origin", exact: true }).click();
    await expect(pane.getByRole("status")).toContainText("Pushed feature/old to origin.");
    expect(git(remote, "for-each-ref", "--format=%(refname:short)", "refs/heads/")).toBe("feature/old");
    // The pushed branch's remote copy is not offered as a push target.
    await row.getByRole("button", { name: /First commit/ }).click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: /origin\/feature\/old to origin/ })).toHaveCount(0);
  } finally {
    await rm(root, { recursive: true, force: true });
    await rm(remote, { recursive: true, force: true });
  }
});
