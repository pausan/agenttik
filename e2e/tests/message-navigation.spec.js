import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

test("prompt arrows visit human messages and stop at both limits", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  const up = page.getByRole("button", { name: "Previous human message", exact: true });
  const down = page.getByRole("button", { name: "Next human message or latest answer", exact: true });
  await expect(up).toBeDisabled();
  await expect(down).toBeDisabled();
  await pickModel(page);
  await sendPrompt(page, "navigation task");
  await expect(page.getByText("idle", { exact: true })).toBeVisible();
  await page.route(/\/api\/sessions\/[^/?]+$/, async route => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    const data = await response.json();
    data.messages = Array.from({ length: 50 }, (_, i) => ({
      ...data.messages[0], id: i + 1, role: [0, 20, 48].includes(i) ? "user" : "assistant",
      content: `Message ${i}\n\nSome content to read.`,
    }));
    await route.fulfill({ response, json: data });
  });
  await expect.poll(() => page.evaluate(() => localStorage.getItem("agenttik.openTabs"))).toContain("session");
  await page.reload();
  await expect(page.locator("[data-human-message]")).toHaveCount(3);
  const transcript = page.locator("main .overflow-auto").filter({ has: page.locator("[data-human-message]") });
  const scrollTop = () => transcript.evaluate(el => el.scrollTop);
  const atMessage = async index => {
    await expect.poll(() => transcript.evaluate((el, index) => {
      const target = el.querySelector(`[data-human-message="${index}"]`);
      const desired = Math.min(el.scrollHeight - el.clientHeight,
        el.scrollTop + target.getBoundingClientRect().top - el.getBoundingClientRect().top - el.clientTop);
      return Math.abs(el.scrollTop - desired);
    }, index)).toBeLessThan(2);
  };
  const draft = page.getByPlaceholder("Ask the agent…");
  await draft.fill("Keep this draft");
  await down.click(); // Already at the end: stay there.
  for (const index of [48, 20, 0, 0]) {
    await up.click();
    await atMessage(index);
  }
  for (const index of [20, 48]) {
    await down.click();
    await atMessage(index);
  }
  await down.click();
  await expect.poll(() => transcript.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
  const bottom = await scrollTop();
  await down.click();
  expect(await scrollTop()).toBe(bottom);
  await up.click();
  await atMessage(48);
  // A manual scroll makes the next click start from the reading position.
  await transcript.evaluate(el => { el.scrollTop = 300; });
  await expect.poll(scrollTop).toBe(300);
  await up.click();
  await atMessage(0);
  await expect(draft).toHaveValue("Keep this draft");
});
