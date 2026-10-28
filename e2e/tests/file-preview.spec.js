import { execFileSync } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test("code previews read-only and a commit's file previews as it was", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-file-preview-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(root, "notes.md"), "# Then\n");
    await writeFile(join(root, "main.go"), "package main\n\nfunc then() {}\n");
    git("add", ".");
    git("commit", "-qm", "Initial files");
    await writeFile(join(root, "notes.md"), "# Now\n");
    await writeFile(join(root, "main.go"), "package main\n\nfunc now() {}\n");
    await addProject(page, root);
    await openProject(page, root);
    const tree = sidebar(page);
    const pane = inspector(page);
    const main = page.getByRole("main");
    const mode = (name) => main.getByRole("tab", { name, exact: true });

    await tree.getByRole("tab", { name: "Tree", exact: true }).click();
    await tree.getByRole("button", { name: "main.go", exact: true }).click();
    await mode("Preview").click();
    await expect(main.locator("pre .hl-k", { hasText: "func" })).toBeVisible();
    await expect(main.locator("pre")).toContainText("func now() {}");
    await expect(main.getByRole("textbox", { name: "main.go", exact: true })).toHaveCount(0);

    await pane.getByRole("tab", { name: "Commits", exact: true }).click();
    await pane.getByRole("button", { name: /Initial files/ }).click();
    await pane.locator('button[data-path="notes.md"]').click();
    await expect(mode("Diff")).toHaveAttribute("aria-selected", "true");
    await expect(mode("Edit")).toHaveCount(0);
    await mode("Preview").click();
    await expect(main.getByRole("heading", { name: "Then", exact: true })).toBeVisible();

    await pane.locator('button[data-path="main.go"]').click();
    await expect(mode("Diff")).toHaveAttribute("aria-selected", "true");
    await mode("Preview").click();
    await expect(main.locator("pre")).toContainText("func then() {}");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
