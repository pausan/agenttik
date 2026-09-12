import { execFileSync } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test("the right workspace selects repositories within one project", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-repos-"));
  try {
    for (const name of ["alpha", "beta"]) {
      const cwd = join(root, name);
      await mkdir(cwd);
      execFileSync("git", ["init"], { cwd });
      await writeFile(join(cwd, "file.txt"), name);
      execFileSync("git", ["add", "."], { cwd });
      execFileSync("git", ["-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", name + " commit"], { cwd });
    }
    await addProject(page, root);
    await openProject(page, root);
    const picker = inspector(page).getByRole("combobox", { name: "Git repository" });
    await expect(picker).toBeVisible();
    await expect(sidebar(page).getByRole("combobox", { name: "Git repository" })).toHaveCount(0);
    await inspector(page).getByRole("tab", { name: "Commits" }).click();
    await expect(inspector(page).getByText("alpha commit", { exact: true })).toBeVisible();
    await picker.click();
    await page.getByRole("option", { name: "beta", exact: true }).click();
    await expect(inspector(page).getByText("beta commit", { exact: true })).toBeVisible();
    await expect(inspector(page).getByText("alpha commit", { exact: true })).toHaveCount(0);
    await inspector(page).getByText("beta commit", { exact: true }).click();
    await inspector(page).getByRole("button", { name: /beta\/file.txt/ }).click();
    await expect(page.getByRole("tab", { name: /file.txt/ })).toBeVisible();
    await addProject(page, join(root, "alpha"));
    await openProject(page, join(root, "alpha"));
    await expect(picker).toHaveCount(0);
    await inspector(page).getByRole("tab", { name: "Commits" }).click();
    await expect(inspector(page).getByText("alpha commit", { exact: true })).toBeVisible();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
