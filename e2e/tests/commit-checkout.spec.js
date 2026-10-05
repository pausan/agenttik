import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, inspector, sidebar, test, expect } from "../fixtures.js";

for (const graph of [false, true]) {
  test(`checkout a commit from the ${graph ? "graph's expanded file" : "list row"} menu`, async ({ page }) => {
    const root = await mkdtemp(join(tmpdir(), "commit-checkout-"));
    const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    try {
      git("init", "-q", "-b", "main");
      git("config", "user.name", "Test");
      git("config", "user.email", "test@example.com");
      await writeFile(join(root, "file.txt"), "first\n");
      git("add", ".");
      git("commit", "-qm", "First commit");
      const first = git("rev-parse", "--short=8", "HEAD");
      await writeFile(join(root, "later.txt"), "second\n");
      git("add", ".");
      git("commit", "-qm", "Second commit");
      const second = git("rev-parse", "--short=8", "HEAD");
      await addProject(page, root);
      await openProject(page, root);
      await sidebar(page).getByRole("tab", { name: "Tree", exact: true }).click();
      await expect(sidebar(page).getByRole("button", { name: "later.txt", exact: true })).toBeVisible();
      const pane = inspector(page);
      await pane.getByRole("tab", { name: "Commits" }).click();
      if (graph) await pane.getByRole("button", { name: "Show commit graph", exact: true }).click();
      const row = pane.locator(`[data-hash="${first}"]`);
      if (graph) {
        await row.getByRole("button", { name: /First commit/ }).click();
        await row.getByRole("button", { name: /file.txt/ }).click({ button: "right" });
      } else {
        await row.getByRole("button", { name: /First commit/ }).click({ button: "right" });
      }
      await page.getByRole("menuitem", { name: "Checkout commit", exact: true }).click();
      const branch = pane.getByRole("button", { name: "Current branch", exact: true });
      await expect(branch).toContainText("detached");
      await expect(row.locator("button[aria-current=true]")).toBeVisible();
      await expect(sidebar(page).getByRole("button", { name: "later.txt", exact: true })).toBeHidden();
      expect(git("rev-parse", "--short=8", "HEAD")).toBe(first);
      expect(git("rev-parse", "--short=8", "main")).toBe(second);
      await branch.click();
      await page.getByRole("option", { name: "main", exact: true }).click();
      await expect(branch).toContainText("main");
      await expect(pane.locator(`[data-hash="${second}"] button[aria-current=true]`)).toBeVisible();
      await expect(sidebar(page).getByRole("button", { name: "later.txt", exact: true })).toBeVisible();
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
}

test("checkout conflicts are reported and local edits survive", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "commit-checkout-conflict-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
  try {
    git("init", "-q", "-b", "main");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(root, "file.txt"), "first\n");
    git("add", ".");
    git("commit", "-qm", "First commit");
    await writeFile(join(root, "file.txt"), "second\n");
    git("commit", "-qam", "Second commit");
    const head = git("rev-parse", "HEAD");
    await writeFile(join(root, "file.txt"), "local edit\n");
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Commits" }).click();
    await pane.getByRole("button", { name: /First commit/ }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Checkout commit", exact: true }).click();
    await expect(pane.getByRole("alert")).toContainText("would be overwritten");
    await expect(pane.getByRole("button", { name: "Current branch", exact: true })).toBeEnabled();
    expect(git("rev-parse", "HEAD")).toBe(head);
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("local edit\n");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
