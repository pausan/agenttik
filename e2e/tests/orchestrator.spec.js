import { execFile } from "node:child_process";
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

import { REPO, addProject, expect, inspector, openProject, openSettings, sidebar, test } from "../fixtures.js";

const run = promisify(execFile);
const binary = fileURLToPath(new URL("../../bin/agenttik-web", import.meta.url));
const settings = (page) => page.getByRole("dialog");
const toggle = (page) => settings(page).getByRole("switch", { name: "Enable orchestrator" });

async function enable(page, agenttik) {
  await openSettings(page, "Orchestrator");
  await expect(toggle(page)).not.toBeChecked();
  await toggle(page).click();
  await expect(toggle(page)).toBeChecked();
  return (await page.request.get(agenttik.url + "/api/orchestrator")).json();
}

async function command(agenttik, method, path, body) {
  const args = ["--data-dir", agenttik.dataDir, "--api", method];
  if (body !== undefined) args.push("--body", JSON.stringify(body));
  args.push(path);
  const { stdout } = await run(binary, args);
  return stdout.trim() ? JSON.parse(stdout) : null;
}

test("orchestrator stays pinned through dragging, API ordering and reload", async ({ page, agenttik }) => {
  await addProject(page, REPO + "/app");
  await addProject(page, REPO + "/web");
  const cfg = await enable(page, agenttik);
  await settings(page).getByRole("button", { name: "Done", exact: true }).click();
  const rows = sidebar(page).locator("[draggable]");
  await expect(rows.first()).toContainText("Orchestrator");
  await expect(rows.first()).toHaveAttribute("draggable", "false");
  await expect(rows.first().getByLabel("Orchestrator · pinned first")).toBeVisible();
  await rows.filter({ hasText: REPO + "/web" }).dragTo(rows.first());
  await expect(rows.first()).toContainText("Orchestrator");

  // New projects are inserted at the top (017-general-settings.md), so web
  // sits above app until the API reverses the whole list — orchestrator
  // included, to prove the server pins it back to the front regardless.
  await expect(rows.nth(1)).toContainText(REPO + "/web");
  const projects = await command(agenttik, "GET", "/api/projects");
  await command(agenttik, "POST", "/api/projects/order", { ids: projects.map((p) => p.id).reverse() });
  await expect(rows.nth(1)).toContainText(REPO + "/app");
  await page.reload();
  await expect(rows.first()).toContainText(cfg.path);
  await expect(rows.nth(1)).toContainText(REPO + "/app");
  await expect(rows.nth(2)).toContainText(REPO + "/web");
});

test("orchestrator prompt resets after a pending edit and from settings", async ({ page, agenttik }) => {
  const cfg = await enable(page, agenttik);
  await settings(page).getByRole("button", { name: "Open project", exact: true }).click();
  await page.getByRole("tab", { name: "Prompt", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "Project prompt" });
  await expect(editor).toHaveValue(cfg.prompt);

  // Hold the blur-save until reset has been clicked; the reset must wait.
  let releaseSave;
  const pendingSave = new Promise((resolve) => { releaseSave = resolve; });
  await page.route(`**/api/projects/${cfg.project_id}`, async (route) => {
    if (route.request().method() === "PATCH") await pendingSave;
    await route.continue();
  });
  await editor.fill("Custom prompt awaiting save");
  await page.getByRole("button", { name: "Reset prompt to default", exact: true }).click();
  releaseSave();
  await expect(editor).toHaveValue(cfg.prompt);
  await page.unroute(`**/api/projects/${cfg.project_id}`);

  await editor.fill("A different custom prompt");
  await editor.blur();
  await expect.poll(async () => (await command(agenttik, "GET", "/api/orchestrator")).prompt).toBe("A different custom prompt");
  await openSettings(page, "Orchestrator");
  await settings(page).getByRole("button", { name: "Reset prompt to default" }).click();
  await expect.poll(async () => (await command(agenttik, "GET", "/api/orchestrator")).prompt).toBe(cfg.prompt);
  await settings(page).getByRole("button", { name: "Done", exact: true }).click();
  await expect(editor).toHaveValue(cfg.prompt);
});

test("disable restores tasks; deletion keeps files and options with fresh history on re-enable", async ({ page, agenttik }) => {
  const cfg = await enable(page, agenttik);
  await settings(page).getByRole("button", { name: "Open project", exact: true }).click();
  const name = inspector(page).getByRole("textbox", { name: "Name" });
  await name.fill("Coordinator");
  await name.blur();
  await expect(sidebar(page).getByText("Coordinator", { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "Prompt", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "Project prompt" });
  await editor.fill("Keep these instructions");
  await editor.blur();
  await expect.poll(async () => (await command(agenttik, "GET", "/api/orchestrator")).prompt).toBe("Keep these instructions");
  await writeFile(join(cfg.path, "notes.md"), "Keep these notes");
  const task = await command(agenttik, "POST", "/api/sessions", {
    project_id: cfg.project_id, provider: "fake", title: "Saved task",
  });

  await openSettings(page, "Orchestrator");
  await toggle(page).click();
  await expect(toggle(page)).not.toBeChecked();
  await expect(sidebar(page).getByText("Coordinator", { exact: true })).toHaveCount(0);
  await toggle(page).click();
  await expect(toggle(page)).toBeChecked();
  expect((await command(agenttik, "GET", "/api/sessions/" + task.id)).session.id).toBe(task.id);
  await settings(page).getByRole("button", { name: "Open project", exact: true }).click();

  await inspector(page).getByRole("button", { name: "Delete project", exact: true }).click();
  await settings(page).getByRole("button", { name: "Delete", exact: true }).click();
  await expect(sidebar(page).getByText("Coordinator", { exact: true })).toHaveCount(0);
  await page.reload();
  await openSettings(page, "Orchestrator");
  await expect(toggle(page)).not.toBeChecked();
  await expect(settings(page).getByText("Coordinator", { exact: true })).toBeVisible();
  await toggle(page).click();
  await expect(toggle(page)).toBeChecked();
  const restored = await command(agenttik, "GET", "/api/orchestrator");
  expect(restored.name).toBe("Coordinator");
  expect(restored.path).toBe(cfg.path);
  expect(restored.prompt).toBe("Keep these instructions");
  expect(await readFile(join(cfg.path, "notes.md"), "utf8")).toBe("Keep these notes");
  expect(await command(agenttik, "GET", `/api/sessions?project_id=${restored.project_id}&window=all&limit=0`)).toEqual([]);
});

test("control commands update live task and project views without reloading", async ({ page, agenttik }) => {
  await addProject(page, REPO + "/app");
  await openProject(page, REPO + "/app");
  const [project] = await command(agenttik, "GET", "/api/projects");
  const task = await command(agenttik, "POST", "/api/sessions", {
    project_id: project.id, provider: "fake", title: "Command-created task",
  });
  await expect(sidebar(page).getByText("Command-created task", { exact: true })).toBeVisible();
  await command(agenttik, "POST", `/api/sessions/${task.id}/messages`, { prompt: "@wait 60000\nA reply" });
  expect((await command(agenttik, "GET", `/api/sessions/${task.id}`)).running).toBe(true);
  await command(agenttik, "POST", `/api/sessions/${task.id}/queue`, { prompt: "Follow-up" });
  expect((await command(agenttik, "GET", `/api/sessions/${task.id}`)).queued).toHaveLength(1);
  await command(agenttik, "POST", `/api/sessions/${task.id}/stop`);
  await expect.poll(async () => (await command(agenttik, "GET", `/api/sessions/${task.id}`)).running).toBe(false);
  expect((await command(agenttik, "GET", `/api/sessions/${task.id}`)).queued).toHaveLength(0);
  await command(agenttik, "PATCH", `/api/sessions/${task.id}`, { done: true });
  await expect(sidebar(page).getByText("Command-created task", { exact: true })).toHaveCount(0);
  await expect(page.locator("main").getByTitle("Unarchive task")).toBeVisible();
  await command(agenttik, "PATCH", `/api/projects/${project.id}`, { name: "Renamed by command" });
  await expect(page.getByRole("heading", { name: "Renamed by command", exact: true })).toBeVisible();
  await command(agenttik, "PATCH", `/api/projects/${project.id}`, { archived: true });
  await expect(sidebar(page).getByText("Renamed by command", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Renamed by command", exact: true })).toHaveCount(0);
  await command(agenttik, "PATCH", `/api/projects/${project.id}`, { archived: false });
  await expect(sidebar(page).getByText("Renamed by command", { exact: true })).toBeVisible();
});

test("orchestrator tasks and jobs use normal views and waiting tasks update on commands", async ({ page, agenttik }) => {
  const cfg = await enable(page, agenttik);
  await settings(page).getByRole("button", { name: "Open project", exact: true }).click();
  const blocker = await command(agenttik, "POST", "/api/sessions", {
    project_id: cfg.project_id, provider: "fake", title: "Blocking task",
  });
  await command(agenttik, "POST", `/api/sessions/${blocker.id}/messages`, { prompt: "@wait 60000" });
  const waiting = await command(agenttik, "POST", "/api/sessions", {
    project_id: cfg.project_id, provider: "fake", title: "Waiting task",
  });
  await sidebar(page).getByText("Waiting task", { exact: true }).click();
  await command(agenttik, "POST", `/api/sessions/${waiting.id}/queue`, { prompt: "Keep this queued instruction" });
  await expect(page.getByLabel("Queued prompt").getByText("Keep this queued instruction")).toBeVisible();
  await command(agenttik, "POST", `/api/sessions/${waiting.id}/stop`);
  await expect(page.getByLabel("Queued prompt")).toHaveCount(0);
  await expect(page.locator(".group.mb-4").getByText("Keep this queued instruction", { exact: true })).toBeVisible();

  await command(agenttik, "POST", "/api/schedules", {
    project_id: cfg.project_id, provider: "fake", title: "Hourly check", prompt: "Check the work",
    every: "interval", interval_minutes: 60, remaining: -1,
  });
  await expect(sidebar(page).getByText("Hourly check", { exact: true })).toBeVisible();
  await openSettings(page, "Orchestrator");
  await expect(toggle(page)).toBeDisabled();
  await command(agenttik, "POST", `/api/sessions/${blocker.id}/stop`);
  await expect(toggle(page)).toBeEnabled();
  await toggle(page).click();
  await expect(toggle(page)).not.toBeChecked();
});

test("reconnecting refreshes a task after its stop event was missed", async ({ page, agenttik }) => {
  // Keep the real EventSource and API. Suppress delivery while disconnected,
  // then signal its reopen so missed events are reconciled deterministically.
  await page.addInitScript(() => {
    const Source = window.EventSource;
    window.EventSource = class extends Source {
      constructor(...args) {
        super(...args);
        window.testStream = this;
        this.addEventListener("message", (event) => {
          if (window.dropStreamEvents) {
            event.stopImmediatePropagation();
            const message = JSON.parse(event.data);
            if (message.event?.type === "done" && message.stats) window.droppedCompletion = true;
            if (message.event?.type === "session_changed") window.droppedTaskChange = true;
          }
        });
      }
    };
  });
  await page.reload();
  await addProject(page, REPO + "/app");
  await openProject(page, REPO + "/app");
  const [project] = await command(agenttik, "GET", "/api/projects");
  const task = await command(agenttik, "POST", "/api/sessions", {
    project_id: project.id, provider: "fake", title: "Reconnect task",
  });
  await sidebar(page).getByText("Reconnect task", { exact: true }).click();
  await command(agenttik, "POST", `/api/sessions/${task.id}/messages`, { prompt: "@wait 60000\nFinish this work" });
  const stop = page.getByRole("button", { name: "Stop", exact: true });
  await expect(stop).toBeVisible();
  await expect.poll(() => page.evaluate(() => window.testStream?.readyState)).toBe(1);
  await page.evaluate(() => { window.dropStreamEvents = true; });
  await command(agenttik, "POST", `/api/sessions/${task.id}/stop`);
  await expect.poll(async () => (await command(agenttik, "GET", `/api/sessions/${task.id}`)).running).toBe(false);
  await expect.poll(() => page.evaluate(() => !!window.droppedCompletion && !!window.droppedTaskChange)).toBe(true);
  await expect(stop).toBeVisible();
  await page.evaluate(() => {
    window.dropStreamEvents = false;
    window.testStream.dispatchEvent(new Event("open"));
  });
  await expect(stop).toBeHidden();
});
