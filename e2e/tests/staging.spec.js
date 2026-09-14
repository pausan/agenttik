import { execFileSync } from "node:child_process";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, newTask, inspector, sidebar, test, expect } from "../fixtures.js";

test("stage, unstage and commit through the expanded composer", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "staging-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(root, "new.txt"), "new file\n");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    const pane = inspector(page);
    await pane.getByRole("button", { name: "Stage new.txt", exact: true }).click();
    await expect(pane.getByRole("button", { name: "Unstage new.txt", exact: true })).toBeVisible();
    await pane.getByRole("button", { name: /^Staged/ }).click();
    await expect(pane.getByRole("button", { name: "Unstage new.txt", exact: true })).toBeHidden();
    await pane.getByRole("button", { name: /^Staged/ }).click();
    await pane.getByRole("button", { name: "Unstage new.txt", exact: true }).click();
    await expect(pane.getByRole("button", { name: "Stage new.txt", exact: true })).toBeVisible();
    await pane.getByRole("button", { name: "Stage all files", exact: true }).click();
    await expect(pane.getByRole("button", { name: "Unstage new.txt", exact: true })).toBeVisible();
    const compact = await pane.getByRole("textbox", { name: "Commit message" }).boundingBox();
    await pane.getByRole("textbox", { name: "Commit message" }).click();
    const dialog = page.getByRole("dialog", { name: "Commit changes" });
    const field = dialog.getByRole("textbox", { name: "Commit message" });
    await expect(field).toBeFocused();
    expect((await field.boundingBox()).width).toBeGreaterThan(compact.width);
    await field.fill("Add new file");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(pane.getByRole("textbox", { name: "Commit message" })).toHaveValue("Add new file");
    await pane.getByRole("textbox", { name: "Commit message" }).click();
    await expect(field).toBeFocused();
    await page.route("**/api/projects/*/commit?*", (route) => route.fulfill({
      status: 400, json: { error: "Commit hook rejected the message" },
    }), { times: 1 });
    await dialog.getByRole("button", { name: "Commit", exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("Commit hook rejected the message");
    await expect(field).toHaveValue("Add new file");
    await dialog.getByRole("button", { name: "Commit", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(pane.getByText("No staged files.")).toBeVisible();
    expect(git("log", "-1", "--format=%s").trim()).toBe("Add new file");
    expect(git("status", "--porcelain")).toBe("");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});


for (const surface of ["Changes", "Tree"]) test(`right-click in ${surface} reverts a changed file only after confirmation`, async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "revert-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(root, "file.txt"), "original\n");
    git("add", ".");
    git("commit", "-m", "Initial");
    await writeFile(join(root, "file.txt"), "changed\n");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    const pane = inspector(page);
    let row = pane.locator("button[data-path='file.txt']");
    if (surface === "Tree") {
      await row.click({ button: "right" });
      await page.getByRole("menuitem", { name: "Show in tree", exact: true }).click();
      row = sidebar(page).getByRole("button", { name: "file.txt", exact: true });
    }
    await row.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Revert changes" }).click();
    const dialog = page.getByRole("dialog", { name: "Revert changes" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("changed\n");
    await row.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Revert changes" }).click();
    await dialog.getByRole("button", { name: "Revert", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(pane.getByText("No edited files.")).toBeVisible();
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("original\n");
    expect(git("status", "--porcelain")).toBe("");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
