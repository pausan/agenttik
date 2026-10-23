import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

for (const surface of ["file tab", "unstaged Changes"]) test(`${surface} renames a file and preserves unsaved edits`, async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-rename-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    await mkdir(join(root, "src"));
    await writeFile(join(root, "src/file.txt"), "original\n");
    await writeFile(join(root, "other.txt"), "other\n");
    git("init", "-q");
    git("add", ".");
    git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "Initial");
    await writeFile(join(root, "src/file.txt"), "changed\n");
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    const tree = sidebar(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    await tree.getByRole("tab", { name: "Tree", exact: true }).click();
    await tree.getByRole("button", { name: "src", exact: true }).click();
    await tree.getByRole("button", { name: "file.txt", exact: true }).dblclick();
    await page.getByRole("textbox", { name: "src/file.txt", exact: true }).fill("unsaved\n");
    // Right clicking a background tab must rename that file, not the active one.
    if (surface === "file tab") await tree.getByRole("button", { name: "other.txt", exact: true }).dblclick();
    const target = surface === "file tab"
      ? page.getByRole("tab", { name: "file.txt", exact: true })
      : pane.locator('button[data-path="src/file.txt"]');
    const ask = async () => {
      await target.click({ button: "right" });
      await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
    };
    await ask();
    const dialog = page.getByRole("dialog", { name: "Rename", exact: true });
    await expect(dialog.getByRole("textbox")).toHaveValue("file.txt");
    await dialog.getByRole("textbox").fill("cancelled.txt");
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(await readFile(join(root, "src/file.txt"), "utf8")).toBe("changed\n");

    await ask();
    // The existing dialog supports moving within a folder and retrying collisions.
    // fail() also logs the handled collision; allow only this deliberate error.
    await page.evaluate(() => {
      const error = console.error.bind(console);
      console.error = (...args) => {
        if (args[0]?.message !== "other.txt already exists") error(...args);
      };
    });
    await dialog.getByRole("textbox").fill("../other.txt");
    await dialog.getByRole("button", { name: "Rename", exact: true }).click();
    await expect(page.getByText("other.txt already exists", { exact: true })).toBeVisible();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("textbox").fill("renamed.txt");
    await dialog.getByRole("button", { name: "Rename", exact: true }).click();
    await expect(dialog).toBeHidden();
    const renamed = page.getByRole("tab", { name: "renamed.txt", exact: true });
    await expect(renamed).toBeVisible();
    await expect(page.getByRole("tab", { name: "file.txt", exact: true })).toHaveCount(0);
    if (surface === "file tab") {
      await expect(page.getByRole("tab", { name: "other.txt", exact: true })).toHaveAttribute("aria-selected", "true");
      await renamed.click();
    }
    await expect(renamed).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("textbox", { name: "src/renamed.txt", exact: true })).toHaveValue("unsaved\n");
    expect(await readFile(join(root, "src/renamed.txt"), "utf8")).toBe("changed\n");
    await expect(pane.getByRole("button", { name: "Stage src/renamed.txt", exact: true })).toBeVisible();
    await expect(tree.getByRole("button", { name: "renamed.txt", exact: true })).toBeVisible();
    expect(git("diff", "--cached", "--name-only")).toBe("");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expect.poll(() => readFile(join(root, "src/renamed.txt"), "utf8")).toBe("unsaved\n");
    await page.reload();
    await expect(renamed).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("textbox", { name: "src/renamed.txt", exact: true })).toHaveValue("unsaved\n");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("rename is unavailable for deleted files, staged lists and historical files", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-rename-history-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    await writeFile(join(root, "file.txt"), "original\n");
    await writeFile(join(root, "deleted.txt"), "removed\n");
    git("init", "-q");
    git("add", ".");
    git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "Initial");
    await writeFile(join(root, "file.txt"), "changed\n");
    git("add", "file.txt");
    await rm(join(root, "deleted.txt"));
    await addProject(page, root);
    await openProject(page, root);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    await pane.locator('button[data-path="file.txt"]').click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: "Show in tree", exact: true })).toBeVisible();
    await expect(page.getByRole("menuitem", { name: "Rename", exact: true })).toHaveCount(0);
    await page.keyboard.press("Escape");
    const deleted = pane.locator('button[data-path="deleted.txt"]');
    await deleted.click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: "Rename", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await deleted.click();
    await page.getByRole("tab", { name: "deleted.txt", exact: true }).click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: "Rename", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await pane.getByRole("tab", { name: "Commits", exact: true }).click();
    await pane.getByRole("button", { name: /Initial/ }).click();
    await pane.locator('button[data-path="file.txt"]').click();
    await page.getByRole("tab", { name: "file.txt", exact: true }).click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: "Rename", exact: true })).toBeDisabled();
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("changed\n");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
