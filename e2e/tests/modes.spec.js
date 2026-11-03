import { addProject, expect, newTask, openProject, openSettings, pickModel, sendPrompt, test } from "../fixtures.js";

/* Modes: a project's prompt saved as a mode, a task opening in another one,
   and a reused blank task going back to its project's. See specs/101-modes.md. */
test("a project prompt becomes a mode and a task picks another before its first prompt", async ({ page, agenttik }) => {
  await addProject(page);
  await openProject(page);
  await page.getByRole("tab", { name: "Prompt", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "Project prompt" });
  await editor.fill("Review only.");
  await editor.blur();
  await page.getByRole("button", { name: "Save as mode…", exact: true }).click();
  await page.getByRole("textbox", { name: "Mode title" }).fill("Review");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(editor).toHaveValue("");
  await expect(page.getByLabel("Mode text")).toHaveText("Review only.");
  await expect(page.getByRole("combobox", { name: "Project mode" })).toHaveText("Review");
  // Edit mode opens Settings on the linked mode.
  await page.getByRole("button", { name: "Edit mode", exact: true }).click();
  await expect(page.getByRole("dialog").getByRole("textbox", { name: "Text of Review" })).toHaveValue("Review only.");
  await page.keyboard.press("Escape");
  const tdd = await (await page.request.post(agenttik.url + "/api/modes", { data: { title: "TDD", body: "Test first." } })).json();

  await newTask(page);
  await pickModel(page);
  const mode = page.getByRole("combobox", { name: "Task mode" });
  await expect(mode).toHaveText("Review (project)");
  await mode.click();
  await page.getByRole("option", { name: "TDD", exact: true }).click();
  await expect(mode).toHaveText("TDD");
  // A New task landing back on this blank one opens it in the project's mode.
  await newTask(page);
  await expect(mode).toHaveText("Review (project)");
  await mode.click();
  await page.getByRole("option", { name: "TDD", exact: true }).click();
  await expect(mode).toHaveText("TDD");

  const sent = page.waitForResponse((r) => r.url().includes("/messages") && r.request().method() === "POST");
  await sendPrompt(page, "hello");
  const id = new URL((await sent).url()).pathname.split("/")[3];
  const { session } = await (await page.request.get(`${agenttik.url}/api/sessions/${id}`)).json();
  expect(session.project_prompt).toBe("Test first.");
  expect(session.mode_title).toBe("TDD");
  await expect(page.getByText("TDD mode added", { exact: true })).toBeVisible();
  // The mode it ran stays named in the prompt bar, out of reach.
  await expect(mode).toHaveText("TDD");
  await expect(mode).toBeDisabled();
  // The chip opens the mode the task ran, not the project's Review.
  await page.getByText("TDD mode added", { exact: true }).click();
  await expect(page.getByRole("dialog").getByRole("textbox", { name: "Text of TDD" })).toHaveValue("Test first.");
  await page.keyboard.press("Escape");

  await openSettings(page, "Modes");
  const dialog = page.getByRole("dialog");
  const pick = dialog.getByRole("combobox", { name: "Mode to edit" });
  await pick.click();
  await page.getByRole("option", { name: "TDD", exact: true }).click();
  const text = dialog.getByRole("textbox", { name: "Text of TDD" });
  await expect(text).toHaveValue("Test first.");
  // The size under the field follows what is typed, before it is saved.
  await expect(dialog.getByText("11 characters · ~3 tokens")).toBeVisible();
  await text.fill("Test first. Then write the code.");
  await expect(dialog.getByText("32 characters · ~8 tokens")).toBeVisible();
  // Over the limit the text stays in the field and nothing is saved.
  await text.fill("y".repeat(32001));
  await expect(dialog.getByText("32,001 characters · ~8.0k tokens · over the 32,000 limit, not saved")).toBeVisible();
  await text.blur();
  await expect(page.getByText("Mode text too long to save")).toBeVisible();
  expect((await (await page.request.get(agenttik.url + "/api/modes")).json()).find((m) => m.title === "TDD").body).toBe("Test first.");
  await text.fill("Test first.");
  await expect(dialog.getByRole("textbox", { name: "Text of Review" })).toHaveCount(0);
  await pick.click();
  await page.getByRole("option", { name: "Review", exact: true }).click();
  await dialog.getByRole("button", { name: "Delete Review" }).click();
  await dialog.getByRole("button", { name: "Delete Review" }).click();
  await expect(dialog.getByRole("textbox", { name: "Text of TDD" })).toBeVisible();
  await expect.poll(async () => (await (await page.request.get(agenttik.url + "/api/modes")).json()).map((m) => m.id)).toEqual([tdd.id]);
});
