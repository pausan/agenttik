import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, inspector, newTask, openProject, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

test("conversation file links reveal nested files and clear the Tree filter", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-transcript-tree-"));
  const path = "src/nested/file.txt";
  try {
    await mkdir(join(root, "src", "nested"), { recursive: true });
    await writeFile(join(root, path), "first\nsecond\nthird\n");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await pickModel(page);
    await sendPrompt(page, `\`${path}:2\` [absolute](${root}/${path}:3) [recover](gone/file.txt)`);

    const tree = sidebar(page);
    for (const [name, line] of [[`${path}:2`, "second"], ["absolute", "third"], ["recover", ""]]) {
      await tree.getByRole("tab", { name: "Tree", exact: true }).click();
      await tree.getByPlaceholder("Filter files").fill("no-match");
      await tree.getByRole("tab", { name: "Projects", exact: true }).click();
      await page.getByRole("button", { name, exact: true }).click({ button: "right" });
      await page.getByRole("menuitem", { name: "Show in tree", exact: true }).click();

      await expect(tree.getByRole("tab", { name: "Tree", exact: true })).toHaveAttribute("data-state", "active");
      await expect(tree.getByPlaceholder("Filter files")).toHaveValue("");
      await expect(tree.getByRole("button", { name: "file.txt", exact: true })).toHaveAttribute("aria-current", "true");
      const editor = page.getByRole("textbox", { name: path, exact: true });
      await expect(editor).toHaveValue("first\nsecond\nthird\n");
      if (line) await expect.poll(() => editor.evaluate((el) => el.value.slice(el.selectionStart, el.selectionEnd))).toBe(line);
      await page.getByRole("tab", { name: "file.txt", exact: true }).getByRole("button", { name: "Close file.txt", exact: true }).click();
    }
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

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
