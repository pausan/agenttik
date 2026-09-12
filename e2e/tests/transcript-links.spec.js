import {
  REPO,
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
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
