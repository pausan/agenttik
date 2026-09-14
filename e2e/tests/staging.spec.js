import { execFileSync } from "node:child_process";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, newTask, inspector, sidebar, test, expect } from "../fixtures.js";

test("stage, unstage and commit through the inline resizable composer", async ({ page }) => {
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
    const field = pane.getByRole("textbox", { name: "Commit message" });
    const commit = pane.getByRole("button", { name: "Commit", exact: true });
    await expect(commit).toBeDisabled();
    await field.click();
    await expect(field).toBeFocused();
    await expect(page.getByRole("dialog")).toBeHidden();
    const box = await field.boundingBox();
    const buttonBox = await commit.boundingBox();
    const stagedBox = await pane.getByRole("button", { name: /^Staged/ }).boundingBox();
    expect(buttonBox.x + buttonBox.width).toBeCloseTo(box.x + box.width, 0);
    expect(buttonBox.y).toBeGreaterThanOrEqual(box.y + box.height);
    expect(buttonBox.y).toBeLessThan(stagedBox.y);
    expect(box.y + box.height).toBeLessThan(stagedBox.y);
    await expect(field).toHaveCSS("resize", "vertical");
    await page.mouse.move(box.x + box.width - 3, box.y + box.height - 3);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width - 3, box.y + box.height + 50, { steps: 5 });
    await page.mouse.up();
    expect((await field.boundingBox()).height).toBeGreaterThan(box.height + 30);
    const generate = pane.getByRole("button", { name: "Generate commit message", exact: true });
    await expect(generate).toHaveAttribute("title", /Does not commit/);
    await expect(commit).toHaveAttribute("title", "Commit staged changes with this message");
    expect((await generate.boundingBox()).x).toBeLessThan((await commit.boundingBox()).x);
    await field.fill("Old draft");
    await page.route("**/api/projects/*/commit-message?*", (route) => route.fulfill({
      json: { message: "Add new file" },
    }), { times: 1 });
    await generate.click();
    await expect(field).toHaveValue("Add new file");
    expect(git("diff", "--cached", "--name-only").trim()).toBe("new.txt");
    expect(() => git("rev-parse", "--verify", "HEAD")).toThrow();
    await page.route("**/api/projects/*/commit-message?*", (route) => route.fulfill({
      status: 400, json: { error: "Could not generate message" },
    }), { times: 1 });
    await generate.click();
    await expect(pane.getByRole("alert")).toHaveText("Could not generate message");
    await expect(field).toHaveValue("Add new file");
    await page.route("**/api/projects/*/commit?*", (route) => route.fulfill({
      status: 400, json: { error: "Commit hook rejected the message" },
    }), { times: 1 });
    await commit.click();
    await expect(pane.getByRole("alert")).toHaveText("Commit hook rejected the message");
    await expect(field).toHaveValue("Add new file");
    await commit.click();
    await expect(field).toHaveValue("");
    await expect(pane.getByText("No staged files.")).toBeVisible();
    expect(git("log", "-1", "--format=%s").trim()).toBe("Add new file");
    expect(git("status", "--porcelain")).toBe("");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});


for (const surface of ["Changes", "Tree", "Changes button", "Staged button"]) test(`${surface} reverts a changed file only after confirmation`, async ({ page }) => {
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
    if (surface === "Staged button") {
      await pane.getByRole("button", { name: "Stage file.txt", exact: true }).click();
      await expect(pane.getByRole("button", { name: "Unstage file.txt", exact: true })).toBeVisible();
    }
    const askRevert = async () => {
      if (surface.endsWith("button")) {
        const revert = pane.getByRole("button", { name: "Revert changes to file.txt", exact: true });
        await expect(revert).toHaveAttribute("title", "Revert changes to file.txt");
        await revert.click();
      } else {
        await row.click({ button: "right" });
        await page.getByRole("menuitem", { name: "Revert changes" }).click();
      }
    };
    await askRevert();
    const dialog = page.getByRole("dialog", { name: "Revert changes" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("changed\n");
    await askRevert();
    await dialog.getByRole("button", { name: "Revert", exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(pane.getByText("No edited files.")).toBeVisible();
    expect(await readFile(join(root, "file.txt"), "utf8")).toBe("original\n");
    expect(git("status", "--porcelain")).toBe("");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
