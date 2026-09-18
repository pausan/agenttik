import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

const tab = (page, name) => page.locator("main").getByRole("tab", { name, exact: true });
const fileScroll = (page) => page.locator(".editor").evaluate(el => el.parentElement.scrollTop);

test("open files restore independent offsets across tabs, modes, and closing", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-scroll-"));
  try {
    for (const name of ["one.md", "two.md"]) {
      await writeFile(join(dir, name), "paragraph\n\n".repeat(300));
    }
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    for (const [name, offset] of [["one.md", 600], ["two.md", 1200]]) {
      await sidebar(page).getByRole("button", { name, exact: true }).dblclick();
      await expect(page.getByRole("textbox", { name, exact: true })).toBeVisible();
      await page.locator(".editor").evaluate((el, top) => { el.parentElement.scrollTop = top; }, offset);
    }
    await tab(page, "one.md").click();
    await expect.poll(() => fileScroll(page)).toBe(600);
    await tab(page, "Preview").click();
    await page.locator("main .markdown").evaluate(el => { el.parentElement.scrollTop = 800; });
    await tab(page, "two.md").click();
    await expect.poll(() => fileScroll(page)).toBe(1200);
    await tab(page, "one.md").click();
    await expect.poll(() => page.locator("main .markdown").evaluate(el => el.parentElement.scrollTop)).toBe(800);
    await tab(page, "Edit").click();
    await expect.poll(() => fileScroll(page)).toBe(600);
    await page.locator(".editor").evaluate(el => { el.parentElement.scrollTop = 0; });
    await tab(page, "two.md").click();
    await tab(page, "one.md").click();
    await expect.poll(() => fileScroll(page)).toBe(0);
    await tab(page, "two.md").click();
    await tab(page, "two.md").getByRole("button", { name: "Close two.md" }).click();
    await sidebar(page).getByRole("button", { name: "two.md", exact: true }).dblclick();
    await expect.poll(() => fileScroll(page)).toBe(0);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("sandboxed HTML keeps its own scroll position while another tab is active", async ({ context, agenttik }) => {
  // Screenshot instrumentation attempts scripts in child frames. The HTML
  // preview deliberately denies those; still fail on every other page error.
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("console", message => {
    if (message.type() === "error" && !message.text().startsWith("Blocked script execution in 'about:srcdoc'")) errors.push(message.text());
  });
  await page.goto(agenttik.url);
  const dir = await mkdtemp(join(tmpdir(), "agenttik-scroll-html-"));
  try {
    await writeFile(join(dir, "page.html"), Array.from({ length: 200 }, (_, i) => `<p>paragraph ${i}</p>`).join(""));
    await addProject(page, dir);
    await openProject(page, dir);
    await newTask(page);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "page.html", exact: true }).dblclick();
    await tab(page, "Preview").click();
    const frame = page.locator('iframe[title="page.html"]');
    await expect(frame).toBeVisible();
    const initial = await frame.screenshot();
    await frame.hover();
    await page.mouse.wheel(0, 700);
    await expect.poll(async () => (await frame.screenshot()).equals(initial)).toBe(false);
    const scrolled = await frame.screenshot();
    await tab(page, "New task").click();
    await tab(page, "page.html").click();
    await expect.poll(async () => (await frame.screenshot()).equals(scrolled)).toBe(true);
    expect(errors).toEqual([]);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});


test("long transcripts restore their reading position instead of jumping to the tail", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-scroll-task-"));
  try {
    await writeFile(join(dir, "file.txt"), "hello");
    await addProject(page, dir);
    await openProject(page, dir);
    await newTask(page);
    await pickModel(page);
    await sendPrompt(page, "scroll task");
    await expect(page.getByText("idle", { exact: true })).toBeVisible();
    await page.route(/\/api\/sessions\/[^/?]+$/, async route => {
      if (route.request().method() !== "GET") return route.continue();
      const response = await route.fetch();
      const data = await response.json();
      data.messages = Array.from({ length: 100 }, (_, i) => ({
        ...data.messages[0], id: i + 1, role: "assistant", content: `Message ${i}\n\nSome content to read.`,
      }));
      await route.fulfill({ response, json: data });
    });
    await expect.poll(() => page.evaluate(() => localStorage.getItem("agenttik.openTabs"))).toContain("session");
    await page.reload();
    await expect(page.getByRole("button", { name: "Copy Agent message", exact: true })).toHaveCount(100);
    const transcript = page.locator("main .overflow-auto").filter({ has: page.getByRole("button", { name: "Copy Agent message", exact: true }) });
    await expect.poll(() => transcript.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
    await transcript.evaluate(el => { el.scrollTop = 400; });
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();
    await sidebar(page).getByRole("button", { name: "file.txt", exact: true }).click();
    await expect(page.getByRole("textbox", { name: "file.txt", exact: true })).toBeVisible();
    await page.locator('main [role="tab"][aria-label*="scroll task"]').click();
    await expect(page.getByRole("button", { name: "Copy Agent message", exact: true })).toHaveCount(100);
    await expect.poll(() => transcript.evaluate(el => el.scrollTop)).toBe(400);
    await transcript.evaluate(el => { el.scrollTop = 0; });
    await tab(page, "file.txt").click();
    await page.locator('main [role="tab"][aria-label*="scroll task"]').click();
    await expect.poll(() => transcript.evaluate(el => el.scrollTop)).toBe(0);
    // Streaming while reading above the bottom must not undo restoration.
    await sendPrompt(page, "@wait 700\nnew output");
    await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeVisible();
    await expect(page.getByText("idle", { exact: true })).toBeVisible();
    await expect.poll(() => transcript.evaluate(el => el.scrollTop)).toBe(0);
    // Returning to the bottom resumes following the next live reply.
    await transcript.evaluate(el => { el.scrollTop = el.scrollHeight; });
    await sendPrompt(page, "@wait 300\nmore output");
    await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeVisible();
    await expect(page.getByText("idle", { exact: true })).toBeVisible();
    await expect.poll(() => transcript.evaluate(el => el.scrollHeight - el.clientHeight - el.scrollTop)).toBeLessThan(2);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
