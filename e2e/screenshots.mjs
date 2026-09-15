// Real UI captures with fictional data. Requires Node 22.13+ (node:sqlite).
import { spawn, execFileSync } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, writeFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { DatabaseSync } from "node:sqlite";
import { chromium, expect } from "@playwright/test";

const repo = fileURLToPath(new URL("..", import.meta.url));
const root = await mkdtemp(join(tmpdir(), "agenttik-showcase-"));
const output = join(repo, "docs/screenshots");
let server, browser, exited;
try {
  await mkdir(output, { recursive: true });
  // Only Git is available to the server: no real provider CLIs or accounts.
  const tools = join(root, "tools");
  await mkdir(tools);
  execFileSync("ln", ["-s", execFileSync("which", ["git"], { encoding: "utf8" }).trim(), join(tools, "git")]);
  server = spawn(join(repo, "bin/agenttik-web"), ["--web", "--private"], {
    env: { ...process.env, PATH: tools, TMPDIR: root, XDG_DATA_HOME: join(root, "data"), AGENTTIK_FAKE_PROVIDER: "1" },
  });
  exited = once(server, "exit");
  let logs = "";
  server.stderr.on("data", data => { logs += data; });
  const url = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Startup timed out: ${logs}`)), 10000);
    server.once("error", error => { clearTimeout(timer); reject(error); });
    server.stdout.on("data", data => {
      const match = /http:\/\/[\d.]+:\d+/.exec(String(data));
      if (match) { clearTimeout(timer); resolve(match[0]); }
    });
  });
  async function post(path, data) {
    const response = await fetch(url + path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data) });
    if (!response.ok) throw new Error(await response.text());
    return response.json();
  }
  const projects = [];
  for (const [name, folder] of [["Orbit Store", "orbit-store"], ["Atlas API", "atlas-api"], ["Field Notes", "field-notes"]]) {
    const path = join(root, folder);
    await mkdir(join(path, "src"), { recursive: true });
    await mkdir(join(path, "docs"));
    await writeFile(join(path, "README.md"), `# ${name}\n\nA fictional project for the agenttik feature showcase.\n`);
    await writeFile(join(path, "docs/release.md"), "# Release checklist\n\n- [x] Review checkout changes\n- [x] Add regression coverage\n- [ ] Review accessibility\n- [ ] Publish release notes\n");
    await writeFile(join(path, "src/shipping.ts"), 'export function shippingTotal(subtotal: number) {\n  return subtotal > 100 ? 0 : 8;\n}\n');
    const git = (...args) => execFileSync("git", args, { cwd: path, stdio: "ignore" });
    git("init", "-b", "feature/free-shipping");
    git("add", ".");
    git("-c", "user.name=Demo Developer", "-c", "user.email=demo@example.com", "commit", "-m", "Add storefront and release checklist");
    await writeFile(join(path, "src/shipping.ts"), 'const FREE_SHIPPING_THRESHOLD = 75;\nconst STANDARD_SHIPPING = 8;\n\nexport function shippingTotal(subtotal: number) {\n  if (subtotal < 0) {\n    throw new RangeError("Subtotal must be non-negative");\n  }\n\n  return subtotal >= FREE_SHIPPING_THRESHOLD\n    ? 0\n    : STANDARD_SHIPPING;\n}\n');
    projects.push(await post("/api/projects", { name, path }));
  }
  const tasks = [];
  for (const [project, titles] of [[projects[0], ["Add free shipping threshold", "Polish the checkout layout", "Review keyboard navigation"]], [projects[1], ["Add cursor pagination", "Document the webhook API"]], [projects[2], ["Build the Markdown preview"]]]) {
    for (const title of titles) tasks.push(await post("/api/sessions", { project_id: project.id, title, provider: "fake", model: "fake-quick", effort: "high" }));
  }
  // Seed a clearly fictional transcript; no paid turn is launched.
  const privateDir = (await readdir(root)).find(name => name.startsWith("agenttik-private-"));
  const db = new DatabaseSync(join(root, privateDir, "agenttik.db"));
  db.prepare("UPDATE server_identity SET name = ? WHERE id = 1").run("Demo workspace");
  const now = Date.now();
  const nextRun = new Date(now);
  nextRun.setUTCHours(9, 0, 0, 0);
  if (nextRun.getTime() <= now) nextRun.setUTCDate(nextRun.getUTCDate() + 1);
  const insert = db.prepare("INSERT INTO messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)");
  insert.run(tasks[0].id, "user", "Offer free shipping on orders of $75 or more. Keep the standard rate at $8 and reject negative subtotals.", now - 120000);
  insert.run(tasks[0].id, "assistant", "## Free shipping is ready\n\nUpdated `src/shipping.ts` with a named threshold and a guard for invalid subtotals.\n\n| Order subtotal | Shipping |\n| --- | --- |\n| $50.00 | $8.00 |\n| $75.00 | Free |\n| $120.00 | Free |\n\n### What changed\n\n- Orders at **$75 or above** qualify for free shipping.\n- Smaller orders keep the **$8 flat rate**.\n- Negative subtotals raise a clear validation error.\n\nThe change is ready for review in the **Changes** panel.\n\n*Demo conversation — no live agent was run.*", now - 110000);
  db.prepare("INSERT INTO schedules (project_id, title, prompt, provider, model, effort, permission, every, at_minute, anchor_at, next_run_at, created_at) VALUES (?, ?, ?, 'fake', 'fake-quick', 'high', 'workspace', 'day', 540, ?, ?, ?)").run(projects[0].id, "Daily quality review", "Review the latest changes for accessibility, edge cases, and missing tests. Summarize findings with file references. Do not modify files.", now, nextRun.getTime(), now);
  db.close();
  browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1440, height: 820 }, deviceScaleFactor: 1, colorScheme: "dark", timezoneId: "UTC" });
  const page = await context.newPage();
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.goto(url);
  const sidebar = page.locator("aside").first();
  const inspector = page.locator("aside").last();
  await expect(page.getByText("Private", { exact: true })).toBeVisible();

  await sidebar.getByText("Orbit Store", { exact: true }).click();
  await sidebar.getByText("Add free shipping threshold", { exact: true }).click();
  await expect(page.getByRole("heading", { name: "Free shipping is ready" })).toBeVisible();
  async function capture(name) {
    await page.mouse.move(720, 0);
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: join(output, name), animations: "disabled" });
  }
  const divider = await page.getByRole("separator", { name: "Resize the left panel" }).boundingBox();
  await page.mouse.move(divider.x + divider.width / 2, 400);
  await page.mouse.down();
  await page.mouse.move(335, 400);
  await page.mouse.up();
  for (let i = 0; i < 2; i++) await sidebar.getByRole("button", { name: "Expand tasks", exact: true }).first().click();
  await capture("workspace.png");
  await inspector.locator('button[data-path="src/shipping.ts"]').click();
  await expect(page.getByRole("tab", { name: "Diff", exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "Diff", exact: true }).click();
  await expect(page.getByText("FREE_SHIPPING_THRESHOLD", { exact: false }).first()).toBeVisible();
  await capture("code-review.png");
  await sidebar.getByText("Daily quality review", { exact: true }).click();
  await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible();
  await capture("schedules.png");
  expect(errors).toEqual([]);
  expect(await page.evaluate(() => Object.keys(localStorage).filter(key => key.startsWith("agenttik.")))).toEqual([]);
  console.log("Captured three screenshots from a fresh private instance and browser context.");
} finally {
  await browser?.close();
  if (server && server.exitCode === null && server.signalCode === null) { server.kill("SIGTERM"); await exited; }
  const leftovers = (await readdir(root)).filter(name => name.startsWith("agenttik-private-"));
  await rm(root, { recursive: true, force: true });
  if (leftovers.length) throw new Error("Private instance did not clean up its data");
}
