import { addProject, expect, newTask, openProject, pickModel, REPO, sendPrompt, test } from "../fixtures.js";

// Bubbles of the conversation itself, not of a thread drawn inside it.
const conversation = (page) => page.locator("[data-message-id]:not([data-threads] [data-message-id])");

test("a passage can be asked about in a thread, which can become a task", async ({ page }) => {
  await addProject(page, REPO);
  await openProject(page, REPO);
  await newTask(page);
  await pickModel(page);
  // The fake agent repeats the prompt's lines, so a reply shows what it was told.
  await sendPrompt(page, "The capital of France is Paris.");
  await expect(page.getByLabel("Turn usage")).toHaveCount(1);
  await expect(conversation(page)).toHaveCount(2);

  // Select part of the agent's reply and ask about it.
  const reply = conversation(page).last();
  await reply.getByText("The capital of France is Paris.").evaluate((p) => {
    const text = p.firstChild;
    const range = document.createRange();
    range.setStart(text, text.textContent.indexOf("Paris"));
    range.setEnd(text, text.textContent.indexOf("Paris") + 5);
    getSelection().removeAllRanges();
    getSelection().addRange(range);
    p.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  });
  await page.getByRole("button", { name: "Ask in thread" }).click();
  const form = page.getByRole("form", { name: "New thread" });
  await expect(form.locator("blockquote")).toHaveText("Paris");
  await form.getByRole("textbox", { name: "Thread question" }).fill("Why Paris?");
  await form.getByRole("textbox", { name: "Thread question" }).press("Enter");

  // The thread knows the conversation and the passage; the conversation is untouched.
  const thread = page.getByRole("region", { name: "Thread: Why Paris?" });
  await expect(thread.getByText("Why Paris?", { exact: true })).toBeVisible();
  await expect(thread.getByLabel("Thread working")).toHaveCount(0);
  await expect(thread).toContainText("User: The capital of France is Paris.");
  await expect(thread).toContainText("<quote>");
  await expect(conversation(page)).toHaveCount(2);
  await expect(page.getByLabel("Turn usage")).toHaveCount(1);

  // A follow-up resumes the thread without repeating its context.
  await thread.getByRole("textbox", { name: "Thread reply" }).fill("And Lyon?");
  await thread.getByRole("textbox", { name: "Thread reply" }).press("Enter");
  await expect(thread.locator("[data-message-id]")).toHaveCount(4);
  await expect(thread.locator("[data-message-id]").last()).toHaveText(/^Agent · \d\d:\d\d:\d\d\s*And Lyon\?\s*$/);

  // Closed, it is a chip that survives a reload.
  await thread.getByRole("button", { name: "Close thread" }).click();
  await expect(page.getByRole("button", { name: "Open thread: Why Paris?" })).toContainText("2 replies");
  await page.reload();
  await page.getByRole("button", { name: "Open thread: Why Paris?" }).click();
  await expect(thread.locator("[data-message-id]")).toHaveCount(4);

  // Opened as a task, it continues with the thread's context; the thread stays.
  await thread.getByRole("button", { name: "Open as task" }).click();
  const back = page.getByRole("button", { name: /Continued from a thread in/ });
  await expect(back).toBeVisible();
  await expect(conversation(page)).toHaveCount(4);
  // Its copies of the thread's prompts have no turn, but they ran there.
  await expect(conversation(page).getByText("Not processed")).toHaveCount(0);
  await sendPrompt(page, "Carry on.");
  await expect(conversation(page).last()).toContainText("side thread about this passage");
  await expect(conversation(page).last()).toContainText("User: And Lyon?");

  await back.click();
  await expect(page.getByRole("region", { name: "Thread: Why Paris?" })).toBeVisible();
  await expect(conversation(page)).toHaveCount(2);
});
