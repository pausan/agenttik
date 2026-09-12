import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, inspector, newTask, openProject, sidebar, test } from "../fixtures.js";

test("Changed and Commits can show a file in the Tree", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-show-tree-"));
  const path = "src/file.txt";
  try {
    await mkdir(join(root, "src"));
    await writeFile(join(root, path), "before\n");
    execFileSync("git", ["init"], { cwd: root });
    execFileSync("git", ["add", "."], { cwd: root });
    execFileSync(
      "git",
      ["-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial"],
      { cwd: root },
    );
    await writeFile(join(root, path), "after\n");

    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);

    const changed = inspector(page).locator(`button[data-path="${path}"]`);
    await expect(changed).toBeVisible();
    await changed.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Show in tree", exact: true }).click();

    const tree = sidebar(page);
    await expect(tree.getByRole("tab", { name: "Tree" })).toHaveAttribute("data-state", "active");
    const treeFile = tree.getByRole("button", { name: "file.txt", exact: true });
    await expect(treeFile).toHaveAttribute("aria-current", "true");
    await expect(page.getByRole("textbox", { name: path, exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "file.txt", exact: true }).getByRole("button", { name: "Close file.txt", exact: true }).click();
    await inspector(page).getByRole("tab", { name: "Commits" }).click();
    await inspector(page).getByText("initial", { exact: true }).click();

    const commitFile = inspector(page).locator(`button[data-path="${path}"]`);
    await expect(commitFile).toBeVisible();
    await commitFile.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Show in tree", exact: true }).click();
    await expect(treeFile).toHaveAttribute("aria-current", "true");
    await expect(page.getByRole("textbox", { name: path, exact: true })).toBeVisible();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
