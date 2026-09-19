import { mkdir } from "node:fs/promises";
import { join } from "node:path";

import {
  REPO,
  addProject,
  expect,
  newTask,
  openProject,
  pickModel,
  sendPrompt,
  sidebar,
  test,
} from "../fixtures.js";

/* What a launch is allowed to cost: the time from the browser opening the
   page to the sidebar, the tab strip and the task in front all being drawn.

   This is a requirement rather than a reading off the machine it runs on —
   see specs/052-launch-budget.md. It is measured on the page's own clock, so
   it counts the script, the requests and the drawing, and nothing of the
   browser that was started to hold them. */
const BUDGET_MS = 1000;

/* Three sibling folders, none of them a prefix of another, so a sidebar row
   is found by its own path and not by the one it sits inside. */
const PROJECTS = [REPO + "/app", REPO + "/web", REPO + "/e2e"];
const named = (path) => path.split("/").pop();
const FILE = "fixtures.js";

/* A strip worth restoring: a project each, a task in each, a conversation in
   the one the launch opens on, and a file beside it. Asking for those one
   after another is what a launch must not do.

   Only the last task is prompted. A turn costs seconds and the suite runs a
   server per test in parallel, so the strip is built from the cheapest thing
   that still has to be fetched per tab. */
async function openAStrip(page) {
  for (const path of PROJECTS) {
    await addProject(page, path);
    await openProject(page, path);
    await newTask(page);
  }
  await pickModel(page);
  await sendPrompt(page, `what does ${named(PROJECTS.at(-1))} do`);
  await expect(page.getByText("idle")).toBeVisible();
  // Opened from the Tree of the project in front, which is the last one.
  await sidebar(page).getByRole("tab", { name: "Tree" }).click();
  await sidebar(page).getByRole("button", { name: FILE }).click();
  await expect(page.getByRole("tab", { name: new RegExp(FILE) })).toBeVisible();
  // Back to the task, so the launch restores a conversation in front with a
  // file tab beside it — what a window is usually left on.
  await sidebar(page).getByRole("tab", { name: "Projects" }).click();
  await sidebar(page).getByText(TITLE).click();
  await expect(page.getByPlaceholder("Ask the agent…")).toBeVisible();
  // The strip is written to localStorage on a debounce, so it has to be on
  // disk before the reload or the launch restores the one before this.
  await expect
    .poll(() => page.evaluate(() => localStorage.getItem("agenttik.openTabs") || ""))
    .toContain(FILE);
}

/* The refined title the fake provider's small model gives the last task, and
   so the label of the tab the launch opens on. See specs/020-task-titles.md. */
const TITLE = `Refined: what does ${named(PROJECTS.at(-1))} do`;

/* Everything a launch has to put on screen: every project in the sidebar, the
   file tab in the strip, and the task in front with its reply. Matched
   against the page's text in lower case, since some of it — the "Agent" over
   a reply — is drawn upper by the stylesheet. */
const DRAWN = [...PROJECTS, FILE, TITLE, "Agent"].map((one) => one.toLowerCase());

/* drawnAt reloads the page and answers with the moment, on the page's own
   clock, that all of DRAWN was on screen. The waiting is done inside the page
   — a frame at a time, rather than across the debugging protocol — so what is
   measured is the launch and not the polling. */
async function drawnAt(page, wanted = DRAWN) {
  await page.reload({ waitUntil: "commit" });
  return page.evaluate(async ({ wanted, task }) => {
    await new Promise((done, reject) => {
      const timeout = setTimeout(() => reject(new Error("Startup content missing after 5s")), 5000);
      const tick = () => {
        const text = document.body.innerText.toLowerCase();
        const taskReady = !task || (
          document.querySelector('main [aria-label="Copy Agent message"]')?.getClientRects().length &&
          document.querySelector('main textarea[placeholder="Ask the agent…"]')?.getClientRects().length
        );
        if (taskReady && wanted.every((one) => text.includes(one))) {
          clearTimeout(timeout);
          requestAnimationFrame(done);
        }
        else requestAnimationFrame(tick);
      };
      tick();
    });
    return performance.now();
  }, { wanted: wanted.map((one) => one.toLowerCase()), task: wanted === DRAWN });
}

test("a launch draws its projects, its strip and the task in front inside the budget", async ({
  page,
}) => {
  await openAStrip(page);

  const ms = await drawnAt(page);
  expect(ms, `the launch took ${Math.round(ms)}ms`).toBeLessThan(BUDGET_MS);
  // The spinner that stands in for a launch still going is never reached by
  // one that keeps to the budget. See App.vue.
  await expect(page.getByLabel("Loading")).toBeHidden();
});

test("a launch reads the two lists once, however many tabs it restores", async ({ page }) => {
  await openAStrip(page);

  const asked = [];
  page.on("request", (r) => {
    const url = new URL(r.url());
    if (url.pathname.startsWith("/api/")) asked.push(url.pathname + url.search);
  });

  await drawnAt(page);
  const launch = asked.slice();

  // The sidebar's two listings are what every restored tab used to re-read on
  // its way in, which made a strip cost a round of requests per tab. They are
  // read once for the whole launch now, and the tabs are fetched together
  // rather than one behind another. See specs/052-launch-budget.md.
  const times = (re) => launch.filter((one) => re.test(one)).length;
  expect(times(/^\/api\/projects$/), `projects read ${times(/^\/api\/projects$/)} times`).toBe(1);
  expect(times(/^\/api\/sessions\?window=[^&]+&include_done=false$/)).toBe(1);
  // One read per restored tab: three tasks and the file beside the last.
  expect(times(/^\/api\/sessions\/[^/?]+$/)).toBe(3);
  expect(times(/^\/api\/projects\/\d+\/file\?/)).toBe(1);
});

// Disable the browser cache: a warm reload alone misses script transfer and
// parsing costs on the first window opened after an update.
async function coldCache(page) {
  const client = await page.context().newCDPSession(page);
  await client.send("Network.enable");
  await client.send("Network.setCacheDisabled", { cacheDisabled: true });
}

async function holdSecondary(page) {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    if (/\/(providers|stars|schedules|stats|metrics|file)$/.test(url.pathname) ||
        url.searchParams.get("only_done") === "true") await gate;
    await route.continue();
  });
  return release;
}

test("cold startup renders tasks while file contents and secondary data are blocked", async ({ page }) => {
  await openAStrip(page);
  await coldCache(page);
  const release = await holdSecondary(page);
  try {
    const ms = await drawnAt(page);
    console.log(`Cold task startup: ${Math.round(ms)}ms`);
    expect(ms).toBeLessThan(BUDGET_MS);
    await expect(page.getByLabel("Loading", { exact: true })).toBeHidden();
    await page.getByRole("tab", { name: new RegExp(FILE) }).click();
    await expect(page.locator("main").getByText("Loading…", { exact: true })).toBeVisible();
  } finally {
    release();
  }
  await expect(page.locator("main").getByText("Loading…", { exact: true })).toBeHidden();
  await expect(page.locator("main")).toContainText("startServer");
});

test("cold startup renders a project and its open tasks before history and statistics", async ({ page }) => {
  await openAStrip(page);
  await openProject(page, PROJECTS.at(-1));
  // beforeunload writes the current strip synchronously.
  await coldCache(page);
  const release = await holdSecondary(page);
  try {
    const ms = await drawnAt(page, [...PROJECTS, FILE, TITLE, "New task"]);
    console.log(`Cold project startup: ${Math.round(ms)}ms`);
    expect(ms).toBeLessThan(BUDGET_MS);
    await expect(page.locator("main").getByText(TITLE, { exact: true })).toBeVisible();
    await expect(page.getByLabel("Loading", { exact: true })).toBeHidden();
  } finally {
    release();
  }
  await page.locator("main").getByRole("tab", { name: "Stats", exact: true }).click();
  await expect(page.locator("main").getByText("Task input tokens", { exact: true })).toBeVisible();
});

test("cold startup loads 30 projects and 300 open tasks within one second", async ({ page, agenttik }) => {
  const projects = [];
  const titles = [];
  for (let i = 0; i < 30; i++) {
    const path = join(agenttik.dataDir, `project-${i}`);
    await mkdir(path);
    const response = await page.request.post(`${agenttik.url}/api/projects`, { data: { path } });
    expect(response.ok()).toBeTruthy();
    const project = await response.json();
    projects.push(project);
    for (let j = 0; j < 10; j++) {
      const title = `Open task ${i}-${j}`;
      titles.push(title);
      const task = await page.request.post(`${agenttik.url}/api/sessions`, {
        data: { project_id: project.id, provider: "fake", model: "fake-quick", title },
      });
      expect(task.ok()).toBeTruthy();
    }
  }
  const tabs = projects.map((p) => ({ id: `project:${p.id}`, kind: "project", projectID: p.id }));
  await page.addInitScript((tabs) => {
    localStorage.setItem("agenttik.foldOthers", "0");
    localStorage.setItem("agenttik.openTabs", JSON.stringify({
      tabs, activeTab: tabs[0].id, activeProjectID: tabs[0].projectID,
    }));
  }, tabs);
  await coldCache(page);
  const ms = await drawnAt(page, [...projects.map((p) => p.path), ...titles]);
  console.log(`Cold 30-project / 300-task startup: ${Math.round(ms)}ms`);
  expect(ms).toBeLessThan(BUDGET_MS);
  await expect(page.locator("main").getByText(titles[0], { exact: true })).toBeVisible();
  await expect(page.getByLabel("Loading", { exact: true })).toBeHidden();
});
