import { execFileSync } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, openSettings, pickModel, test } from "../fixtures.js";

test.beforeEach(async ({ page }) => {
  await expect(page.getByRole("button", { name: "Take the Quick Start Tour", exact: true })).toBeVisible();
  await expect(tour(page)).toHaveCount(0);
  await page.getByRole("button", { name: "Take the Quick Start Tour", exact: true }).click();
});

const tour = (page) => page.getByRole("region", { name: "Quick Start Tour", exact: true });
async function goTo(page, title) {
  for (let i = 0; i < 12; i++) {
    if (await tour(page).getByRole("heading", { name: title, exact: true }).isVisible()) return;
    await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  }
  throw new Error(`Tour step not found: ${title}`);
}

test("manual launch, free navigation, resume, dismissal and Help restart", async ({ page }) => {
  await expect(tour(page)).toBeVisible();
  await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  await expect(tour(page)).toContainText("Already connected: Fake · System");
  await page.reload();
  await expect(tour(page)).toContainText("Set up your first agent");
  await tour(page).getByRole("button", { name: "Previous" }).click();
  await expect(tour(page)).toContainText("Learn by building a calculator");
  await tour(page).getByRole("button", { name: "Close tour" }).click();
  await page.reload();
  await openSettings(page, "Help");
  await expect(tour(page)).toHaveCount(0);
  await page.getByRole("button", { name: "Start Quick Start Tour" }).click();
  await expect(tour(page)).toContainText("Learn by building a calculator");
  await goTo(page, "Take this workflow to your projects");
  await tour(page).getByRole("button", { name: "Finish tour" }).click();
  await page.reload();
  await expect(page.getByRole("button", { name: "Settings", exact: true })).toBeVisible();
  await expect(tour(page)).toHaveCount(0);
});

test("setup dialogs and tour remain independently usable; copying never submits", async ({ page, agenttik }) => {
  const mutations = [];
  page.on("request", (r) => { if (r.method() === "POST") mutations.push(r.url()); });
  await goTo(page, "Set up your first agent");
  await openSettings(page, "Subscriptions");
  await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Settings", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Done", exact: true }).click();
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await goTo(page, "Give your agent a way to work");
  const prompt = page.getByPlaceholder("Ask the agent…");
  await prompt.fill("my draft");
  await expect(tour(page).getByRole("button", { name: "Copy to prompt area" })).toBeDisabled();
  await expect(prompt).toHaveValue("my draft");
  await prompt.fill("");
  mutations.length = 0;
  await tour(page).getByRole("button", { name: "Copy to prompt area" }).click();
  await expect(prompt).toHaveValue(/Create AGENTS.md/);
  expect(mutations).toEqual([]);
  const sessions = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
  expect(sessions.every((s) => s.status !== "running" && !s.queue_count)).toBe(true);
  await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  await expect(prompt).toHaveValue(/Create AGENTS.md/);
});

test("tour fits a phone, minimizes and closes while a setup dialog is open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(tour(page)).toBeVisible();
  const box = await tour(page).boundingBox();
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(390);
  expect(box.y + box.height).toBeLessThanOrEqual(844);
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  await expect(tour(page).getByRole("heading")).toHaveCount(0);
  await tour(page).getByRole("button", { name: "Expand tour" }).click();
  await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Add project", exact: true })).toBeVisible();
  await tour(page).getByRole("button", { name: "Close tour" }).click();
  await expect(tour(page)).toHaveCount(0);
});

test("clone locally, enqueue the tutorial prompts and send a parallel question", async ({ page, agenttik }, testInfo) => {
  const source = join(agenttik.dataDir, "local sources", "agenttik-helloworld");
  const destination = join(agenttik.dataDir, "practice");
  await mkdir(source, { recursive: true });
  await mkdir(destination);
  await writeFile(join(source, "main.py"), 'print("Hello, world!")\n');
  execFileSync("git", ["init", "-b", "main", source]);
  execFileSync("git", ["-C", source, "add", "."]);
  execFileSync("git", ["-C", source, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "Hello world"]);
  await goTo(page, "Create a practice project");
  await page.getByRole("button", { name: "Add project", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add project", exact: true });
  await dialog.getByRole("tab", { name: "Multiple Git Repos" }).click();
  const folder = dialog.getByPlaceholder("~/code/myproject");
  await expect(folder).toHaveValue(/.+/);
  await folder.fill(destination);
  const repository = dialog.getByRole("textbox", { name: "Repository 1", exact: true });
  const intercepted = await repository.evaluate((input, path) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", path);
    const paste = new ClipboardEvent("paste", { clipboardData, bubbles: true, cancelable: true });
    input.dispatchEvent(paste);
    return paste.defaultPrevented;
  }, source);
  expect(intercepted).toBe(false);
  await repository.fill(source);
  await expect(dialog.getByRole("button", { name: "Add project", exact: true })).toBeEnabled();
  await dialog.getByRole("button", { name: "Add project", exact: true }).click();
  await expect(dialog).toBeHidden();
  await openProject(page, destination);
  await newTask(page);
  await pickModel(page);
  const prompt = page.getByPlaceholder("Ask the agent…");
  await goTo(page, "Give your agent a way to work");
  for (let i = 0; i < 4; i++) {
    await tour(page).getByRole("button", { name: "Copy to prompt area" }).click();
    // The fake provider does not execute Python; its wait directive stands in
    // for the first prompt's sleep while the next three are enqueued.
    if (i === 0) await prompt.fill((await prompt.inputValue()) + "\n@wait 20000");
    const queued = page.waitForResponse((r) => r.request().method() === "POST" && /\/queue$/.test(r.url()));
    await page.getByRole("button", { name: "More prompt actions" }).click();
    await page.getByRole("button", { name: "Enqueue", exact: true }).last().click();
    expect((await queued).ok()).toBe(true);
    await expect(prompt).toHaveValue("");
    if (i < 3) await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  }
  const sessions = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
  expect(sessions).toHaveLength(1);
  expect(sessions[0].queue_count).toBe(3);
  expect(execFileSync("git", ["-C", join(destination, "agenttik-helloworld"), "status", "--porcelain"], { encoding: "utf8" })).toBe("");
  await page.screenshot({ path: testInfo.outputPath("quick-start-desktop.png") });
  await tour(page).getByRole("button", { name: "Next", exact: true }).click();
  await newTask(page);
  await tour(page).getByRole("button", { name: "Copy to prompt area" }).click();
  await page.getByRole("button", { name: "More prompt actions" }).click();
  await page.getByRole("button", { name: "Send", exact: true }).last().click();
  await expect(prompt).toHaveValue("");
  // The prompt box clears as soon as the send starts, so the second task is
  // still being created on the server for a moment after it empties.
  await expect.poll(async () => {
    const parallel = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
    return parallel.filter((s) => s.status === "running").length;
  }).toBe(2);
});
