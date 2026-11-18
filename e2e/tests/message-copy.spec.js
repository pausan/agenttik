import { addProject, expect, newTask, openProject, pickModel, REPO, sendPrompt, test } from "../fixtures.js";

test("right-click copies selected text in both prompts and replies", async ({ page, context }) => {
  await addProject(page, REPO);
  await openProject(page, REPO);
  await newTask(page);
  await pickModel(page);
  await sendPrompt(page, "Copy only this passage.");
  await expect(page.getByLabel("Turn usage")).toHaveCount(1);
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);

  const messages = page.locator("[data-message-id]");
  for (const message of [messages.first(), messages.last()]) {
    const text = message.getByText("Copy only this passage.", { exact: true });
    const bounds = await text.evaluate((el) => {
      const range = document.createRange();
      range.setStart(el.firstChild, 5);
      range.setEnd(el.firstChild, 14);
      const rect = range.getBoundingClientRect();
      return { left: rect.left, right: rect.right, y: rect.top + rect.height / 2 };
    });

    await page.mouse.move(bounds.left, bounds.y);
    await page.mouse.down();
    await page.mouse.move(bounds.right, bounds.y, { steps: 5 });
    await page.mouse.up();

    await page.mouse.click((bounds.left + bounds.right) / 2, bounds.y, { button: "right" });
    const copy = page.getByRole("menuitem", { name: "Copy", exact: true });
    await expect(copy).toBeVisible();
    expect(await page.evaluate(() => window.getSelection().toString())).toBe("only this");
    await copy.click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("only this");
    await page.evaluate(() => navigator.clipboard.writeText(""));
  }
});
