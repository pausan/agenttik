import { addProject, expect, newTask, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

test("project pause waits for completion and resumes from a task menu", async ({ page, agenttik }) => {
  await addProject(page);
  const project = sidebar(page).locator(".sidebar-project").last();
  const pause = project.getByRole("button", { name: "Pause project queue" });
  const resume = project.getByRole("button", { name: "Resume project queue" });
  await expect(pause).toHaveCount(0);
  await expect(resume).toHaveCount(0);
  await newTask(page);
  await pickModel(page, "Fake Quick");
  await sendPrompt(page, "@wait 6000\ncurrent work finished");
  await expect(pause).toBeVisible();
  await expect(pause).toHaveAttribute("title", "Pause queued work after active tasks finish");
  const pauseBox = await pause.boundingBox();
  const hideBox = await project.getByRole("button", { name: "Hide project" }).boundingBox();
  expect(pauseBox.x).toBeLessThan(hideBox.x);

  const [session] = await (await page.request.get(`${agenttik.url}/api/sessions`)).json();
  const detailURL = `${agenttik.url}/api/sessions/${session.id}`;
  expect((await page.request.post(`${detailURL}/queue`, { data: { prompt: "queued work finished" } })).ok()).toBeTruthy();
  await sidebar(page).locator(".task-row").first().click({ button: "right" });
  await expect(page.getByRole("menuitem", { name: "Pause project queue" })).toBeVisible();
  await page.keyboard.press("Escape");
  await pause.click();
  await expect(resume).toBeVisible();
  expect((await (await page.request.get(detailURL)).json()).session.status).toBe("running");
  await expect.poll(async () => (await (await page.request.get(detailURL)).json()).session.status).toBe("idle");
  const detail = await (await page.request.get(detailURL)).json();
  expect(detail.queued).toHaveLength(1);
  expect(detail.messages.some((m) => m.role === "assistant" && m.content.includes("current work finished"))).toBeTruthy();

  await page.reload();
  await expect(resume).toBeVisible();
  await sidebar(page).locator(".task-row").first().click({ button: "right" });
  await page.getByRole("menuitem", { name: "Resume project queue" }).click();
  await expect.poll(async () => {
    const d = await (await page.request.get(detailURL)).json();
    return d.queued.length === 0 && d.session.status === "idle";
  }).toBeTruthy();
  await expect(pause).toHaveCount(0);
  await expect(resume).toHaveCount(0);
  await expect(page.getByText("queued work finished", { exact: true }).last()).toBeVisible();
});
