import { execFileSync } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { addProject, expect, inspector, openProject, sidebar, test } from "../fixtures.js";

test("file clicks choose Diff in Changes and Commits and prefer Edit in Tree", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-file-opening-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });
  try {
    git("init", "-q");
    git("config", "user.name", "Test");
    git("config", "user.email", "test@example.com");
    await writeFile(join(root, "readme.md"), "# Original\n");
    await writeFile(join(root, "deleted.txt"), "removed later\n");
    await writeFile(join(root, "picture.png"), Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=", "base64"));
    git("add", ".");
    git("commit", "-qm", "Initial files");
    await writeFile(join(root, "readme.md"), "# Changed\n");
    await rm(join(root, "deleted.txt"));
    await addProject(page, root);
    await openProject(page, root);
    const tree = sidebar(page);
    const pane = inspector(page);
    await pane.getByRole("tab", { name: "Changed", exact: true }).click();
    const mode = (name) => page.getByRole("tab", { name, exact: true });
    const selected = (name) => expect(mode(name)).toHaveAttribute("aria-selected", "true");
    const changed = (path) => pane.locator(`button[data-path="${path}"]`);
    await tree.getByRole("tab", { name: "Tree", exact: true }).click();
    const readme = tree.getByRole("button", { name: "readme.md", exact: true });

    await readme.click();
    await selected("Edit");
    await mode("Preview").click();
    await readme.click();
    await selected("Edit");
    // The same kept tab switches views without losing its unsaved text.
    const editor = page.getByRole("textbox", { name: "readme.md", exact: true });
    await editor.fill("# Unsaved\n");
    await changed("readme.md").click();
    await selected("Diff");
    await readme.click();
    await selected("Edit");
    await expect(editor).toHaveValue("# Unsaved\n");

    // A new deleted-file tab must fetch its diff, not its missing contents.
    await changed("deleted.txt").dblclick();
    await selected("Diff");
    await expect(page.getByText("-removed later", { exact: true })).toBeVisible();
    await tree.getByRole("button", { name: "picture.png", exact: true }).click();
    await selected("Preview");
    await expect(mode("Edit")).toHaveCount(0);
    await readme.dblclick();
    await selected("Edit");
    await changed("readme.md").dblclick();
    await selected("Diff");

    await readme.click();
    await selected("Edit");
    await pane.getByRole("tab", { name: "Commits", exact: true }).click();
    await pane.getByRole("button", { name: /Initial files/ }).click();
    await changed("readme.md").click();
    await selected("Diff");
    await expect(mode("Edit")).toHaveCount(0);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
