import { execFileSync } from "node:child_process";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, openProject, sidebar, test, expect } from "../fixtures.js";

test("diff context persists across files and reloads, independently of layout", async ({ page, agenttik }) => {
  const root = await mkdtemp(join(tmpdir(), "diff-context-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q");
    const original = Array.from({ length: 30 }, (_, i) => `unchanged line ${i + 1}`).join("\n") + "\n";
    for (const name of ["first.txt", "second.txt"]) await writeFile(join(root, name), original);
    git("add", ".");
    git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "Initial");
    const changed = original.replace("unchanged line 15", "changed middle");
    for (const name of ["first.txt", "second.txt"]) await writeFile(join(root, name), changed);
    git("add", ".");
    git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "Change middle");
    const hash = git("rev-parse", "HEAD").trim();
    for (const name of ["first.txt", "second.txt"]) await writeFile(join(root, name), changed.replace("changed middle", "working change"));
    await addProject(page, root);
    await openProject(page, root);
    await sidebar(page).getByRole("tab", { name: "Tree", exact: true }).click();
    const main = page.getByRole("main");
    const openDiff = async (name) => {
      await sidebar(page).getByRole("tab", { name: "Tree", exact: true }).click();
      await sidebar(page).getByRole("button", { name, exact: true }).click();
      await main.getByRole("tab", { name: "Diff", exact: true }).click();
    };
    await openDiff("first.txt");
    await expect(main.getByText("unchanged line 1", { exact: true })).toHaveCount(0);
    await main.getByRole("tab", { name: "Whole file", exact: true }).click();
    await expect(main.getByText("unchanged line 1", { exact: true })).toBeVisible();
    await expect(main.locator("pre")).toContainText("+working change");
    await main.getByRole("tab", { name: "Split", exact: true }).click();
    await expect(main.getByText("unchanged line 30", { exact: true })).toHaveCount(2);
    await openDiff("second.txt");
    await expect(main.getByRole("tab", { name: "Whole file", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(main.getByText("unchanged line 30", { exact: true })).toHaveCount(2);
    await page.reload();
    await expect(main.getByRole("tab", { name: "Whole file", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(main.getByText("unchanged line 30", { exact: true })).toHaveCount(2);
    await main.getByRole("tab", { name: "Changes", exact: true }).click();
    await expect(main.getByText("unchanged line 30", { exact: true })).toHaveCount(0);
    await openDiff("first.txt");
    await expect(main.getByRole("tab", { name: "Changes", exact: true })).toHaveAttribute("aria-selected", "true");
    // The full-context commit endpoint must use the historical revision.
    const projects = await (await page.request.get(agenttik.url + "/api/projects")).json();
    const project = projects.find((p) => p.path === root);
    const base = `${agenttik.url}/api/projects/${project.id}/commit/diff?hash=${hash}&path=first.txt`;
    const compact = await (await page.request.get(base)).json();
    const full = await (await page.request.get(base + "&context=full")).json();
    expect(compact.diff).not.toContain("unchanged line 30");
    expect(full.diff).toContain("unchanged line 30");
    expect(full.diff).toContain("+changed middle");
    expect(full.diff).not.toContain("working change");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
