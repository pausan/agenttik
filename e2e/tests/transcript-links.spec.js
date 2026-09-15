import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  REPO,
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  sidebar,
  test,
} from "../fixtures.js";

test("a transcript web link can be copied or opened from its menu", async ({ page, context }) => {
  await addProject(page, REPO);
  await openProject(page, REPO);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "[example](https://example.com/docs)");

  const link = page.getByRole("link", { name: "example" });
  await expect(link).toHaveAttribute("href", "https://example.com/docs");
  await expect(link).toHaveAttribute("target", "_blank");

  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: new URL(page.url()).origin,
  });
  await link.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Copy link" }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("https://example.com/docs");

  await page.evaluate(() => {
    window.openedLinks = [];
    window.open = (url) => {
      window.openedLinks.push(String(url));
      return null;
    };
  });
  await link.click({ button: "right" });
  await page.getByRole("menuitem", { name: "Open in browser" }).click();
  await expect.poll(() => page.evaluate(() => window.openedLinks)).toEqual(["https://example.com/docs"]);
});

test("local transcript links open kept tabs and offer the system opener", async ({ page, context }) => {
  await addProject(page, REPO);
  await openProject(page, REPO);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, `[first](README.md) [second](Makefile) [absolute](${REPO}/README.md:3) [missing](no-such-file.md) [directory](web/) [root](.)`);

  const opened = [];
  await page.route("**/api/projects/*/open", async (route) => {
    opened.push(route.request().postDataJSON());
    await route.fulfill({ json: { path: "README.md", dir: false } });
  });
  await page.getByRole("button", { name: "absolute", exact: true }).click({ button: "right" });
  await page.getByRole("menuitem", { name: "Open in system browser" }).click();
  await expect.poll(() => opened).toEqual([{ path: "README.md" }]);

  await context.grantPermissions(["clipboard-read", "clipboard-write"], {
    origin: new URL(page.url()).origin,
  });
  for (const [name, path] of [["absolute", "README.md"], ["directory", "web"], ["root", "."]]) {
    await page.getByRole("button", { name, exact: true }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Copy path", exact: true }).click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(path);
  }
  await page.getByRole("button", { name: "directory", exact: true }).click();
  await page.getByRole("button", { name: "root", exact: true }).click();
  await expect.poll(() => opened).toEqual([{ path: "README.md" }, { path: "web" }, { path: "." }]);
  await expect(page.locator('[role="tab"][title="web"]')).toHaveCount(0);

  // fail() logs handled API errors too; allow only this deliberate failure.
  await page.evaluate(() => {
    const error = console.error.bind(console);
    console.error = (...args) => {
      if (!args[0]?.message?.startsWith("cannot read no-such-file.md:")) error(...args);
    };
  });
  // A missing path reports the API failure and leaves the transcript in place.
  await page.getByRole("button", { name: "missing", exact: true }).click();
  await expect(page.getByText(/^cannot read no-such-file.md:/)).toBeVisible();
  await expect(page.getByRole("button", { name: "first", exact: true })).toBeVisible();
  await expect(page.locator('[role="tab"][title="no-such-file.md"]')).toHaveCount(0);

  await page.getByRole("button", { name: "first", exact: true }).click();
  const firstTab = page.getByRole("tab", { name: "README.md", exact: true });
  await expect(firstTab).toBeVisible();
  await expect(firstTab).not.toHaveAttribute("title", /temporary/i);
  // Closing returns to the task; the second action exercises the file menu.
  await firstTab.getByRole("button", { name: "Close README.md", exact: true }).click();
  await page.getByRole("button", { name: "second", exact: true }).click({ button: "right" });
  await page.getByRole("menuitem", { name: "Open in new tab" }).click();
  await expect(page.getByRole("tab", { name: "Makefile", exact: true })).toBeVisible();
});

test("Markdown preview links resolve from their document and keep existing tabs", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-markdown-links-"));
  try {
    await mkdir(join(dir, "docs"));
    await writeFile(join(dir, "docs", "index.md"), "[sibling](my%20file.md) [parent](../README.md)");
    await writeFile(join(dir, "docs", "my file.md"), "# Sibling document");
    await writeFile(join(dir, "README.md"), "# Parent document");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    // Folders start collapsed, so the document lives one click down.
    await sidebar(page).getByRole("button", { name: "docs", exact: true }).click();
    await sidebar(page).getByRole("button", { name: "index.md", exact: true }).click();
    await page.getByRole("tab", { name: "Preview", exact: true }).click();

    const opened = [];
    await page.route("**/api/projects/*/open", async (route) => {
      opened.push(route.request().postDataJSON());
      await route.fulfill({ json: { path: "docs/my file.md", dir: false } });
    });
    await page.getByRole("button", { name: "sibling", exact: true }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Open in system browser" }).click();
    await expect.poll(() => opened).toEqual([{ path: "docs/my file.md" }]);
    await page.getByRole("button", { name: "sibling", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Sibling document" })).toBeVisible();
    const siblingTab = page.getByRole("tab", { name: "my file.md", exact: true });
    await expect(siblingTab).not.toHaveAttribute("title", /temporary/i);
    await page.getByRole("tab", { name: "index.md", exact: true }).click();
    await page.getByRole("button", { name: "parent", exact: true }).click({ button: "right" });
    await page.getByRole("menuitem", { name: "Open in new tab" }).click();
    await expect(page.getByRole("heading", { name: "Parent document" })).toBeVisible();
    await expect(siblingTab).toBeVisible();
    await expect(page.getByRole("tab", { name: "index.md", exact: true })).toBeVisible();
    await expect(page.getByRole("tab", { name: "README.md", exact: true })).not.toHaveAttribute("title", /temporary/i);
    await page.getByRole("tab", { name: "index.md", exact: true }).click();
    await page.getByRole("button", { name: "parent", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Parent document" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "README.md", exact: true })).toHaveCount(1);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
