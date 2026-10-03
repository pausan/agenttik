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

for (const mobile of [false, true]) {
  test(`sticky prompts stay compact and return to the original on ${mobile ? "mobile" : "desktop"}`, async ({ page }) => {
    await addProject(page);
    await openProject(page);
    await newTask(page);
    await pickModel(page);
    await sendPrompt(page, "sticky prompt task");
    await expect(page.getByText("idle", { exact: true })).toBeVisible();
    // The last prompt wraps even without explicit newlines. Earlier prompts
    // also check short text and blank lines in the compact copy.
    const lastPrompt = "Last human prompt " + "Read this detailed instruction carefully. ".repeat(35);
    await page.route(/\/api\/sessions\/[^/?]+$/, async route => {
      if (route.request().method() !== "GET") return route.continue();
      const response = await route.fetch();
      const data = await response.json();
      data.messages = Array.from({ length: 60 }, (_, i) => ({
        ...data.messages[0], id: i + 1, role: [0, 20, 40].includes(i) ? "user" : "assistant",
        content: i === 40 ? lastPrompt : i === 20 ? "Earlier short prompt" : i === 0
          ? "First prompt\n\nThird line\nFourth line\nFifth line"
          : `Reply ${i}\n\n${"Some content to read.\n\n".repeat(4)}`,
      }));
      await route.fulfill({ response, json: data });
    });
    await expect.poll(() => page.evaluate(() => localStorage.getItem("agenttik.openTabs"))).toContain("session");
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    await page.reload();
    await expect(page.locator("[data-human-message]")).toHaveCount(3);
    const transcript = page.locator("main .overflow-auto").filter({ has: page.locator("[data-human-message]") });
    const pinned = page.getByLabel("Pinned human prompt", { exact: true });
    const pinnedText = pinned.locator(".line-clamp-3");
    await expect(pinnedText).toHaveText(lastPrompt);
    const showMore = pinned.getByText("Show more", { exact: true });
    await expect(showMore).toBeVisible();
    const textBox = await pinnedText.boundingBox();
    const labelBox = await showMore.boundingBox();
    expect(labelBox.y).toBeGreaterThan(textBox.y + textBox.height / 2);
    expect(labelBox.y + labelBox.height).toBeLessThanOrEqual(textBox.y + textBox.height + 1);
    expect(labelBox.x + labelBox.width).toBeLessThanOrEqual(textBox.x + textBox.width + 1);
    expect(await pinnedText.evaluate(el => el.clientHeight / parseFloat(getComputedStyle(el).lineHeight))).toBeLessThanOrEqual(3.1);
    expect(await pinned.evaluate(el => el.getBoundingClientRect().top) - await transcript.evaluate(el => el.getBoundingClientRect().top)).toBeLessThan(2);

    const scrollPast = async (index, distance) => {
      await transcript.evaluate((el, { index, distance }) => {
        const node = el.querySelector(`[data-human-message="${index}"]`);
        el.scrollTop += node.getBoundingClientRect().bottom - el.getBoundingClientRect().top - el.clientTop + distance;
      }, { index, distance });
    };
    await showMore.click();
    await expect(pinned).toBeHidden();
    await expect.poll(() => transcript.evaluate(el => Math.abs(
      el.querySelector('[data-human-message="40"]').getBoundingClientRect().top - el.getBoundingClientRect().top - el.clientTop,
    ))).toBeLessThan(2);
    await scrollPast(40, 10);
    await expect(pinned).toBeHidden();
    const height = await transcript.evaluate(el => el.clientHeight);
    await scrollPast(40, height / 2 + 10);
    await expect(pinnedText).toHaveText(lastPrompt);

    await scrollPast(20, 10);
    await expect(pinnedText).toHaveText("Earlier short prompt");
    await expect(showMore).toHaveCount(0);
    await scrollPast(0, 10);
    await expect(pinnedText).toHaveText("First prompt\n\nThird line\nFourth line\nFifth line");
    await expect(showMore).toBeVisible();
    await transcript.evaluate(el => { el.scrollTop = 0; });
    await expect(pinned).toBeHidden();
  });
}
