import { spawn } from "node:child_process";
import { access, constants, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test as base } from "@playwright/test";

const BIN = fileURLToPath(new URL("../bin/agenttik-web", import.meta.url));

/* A machine that happens to have claude, codex or copilot installed makes the
   app probe a real CLI at startup — sometimes for seconds, once each for
   however many servers a parallel run has up at once — for a provider no test
   here ever asks for. computeTestPath drops whatever directory on PATH holds
   one of those CLIs, so the server this suite spawns only ever finds the
   fake provider ready, regardless of what the host otherwise has installed. */
const REAL_PROVIDER_CLIS = ["claude", "codex", "copilot", "opencode"];

let testPathPromise;
function testPath() {
  if (!testPathPromise) testPathPromise = computeTestPath();
  return testPathPromise;
}

async function computeTestPath() {
  const dirs = (process.env.PATH || "").split(delimiter).filter(Boolean);
  const hasRealCLI = await Promise.all(
    dirs.map((dir) =>
      Promise.all(REAL_PROVIDER_CLIS.map((bin) => access(join(dir, bin), constants.X_OK).then(() => true, () => false))).then(
        (found) => found.some(Boolean),
      ),
    ),
  );
  return dirs.filter((_, i) => !hasRealCLI[i]).join(delimiter);
}

/* Every test gets its own server, on its own port, over its own database, so
   nothing one test adds is visible to another and they can run in parallel.
   Port 0 lets the OS pick; the server prints the URL it settled on. */
async function startServer() {
  try {
    await access(BIN);
  } catch {
    throw new Error(`${BIN} is missing — run \`make build-web\` first (or \`make e2e\`).`);
  }

  const dataDir = await mkdtemp(join(tmpdir(), "agenttik-e2e-"));
  // AGENTTIK_FAKE_PROVIDER registers the scriptable "Fake" provider (see
  // app/internal/agent/fake) so a turn can be driven without a CLI on PATH or
  // a real subscription. A normal launch never sets this, so it never shows
  // up outside this suite.
  const proc = spawn(BIN, ["--web", "--addr", "127.0.0.1:0", "--data-dir", dataDir], {
    env: { ...process.env, AGENTTIK_FAKE_PROVIDER: "1", PATH: await testPath(), XDG_DATA_HOME: join(dataDir, "xdg-data") },
  });
  const stderr = [];
  proc.stderr.on("data", (b) => stderr.push(String(b)));

  const url = await new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error("agenttik did not print a URL in 10s: " + stderr.join(""))),
      10_000,
    );
    proc.on("error", reject);
    proc.on("exit", (code) => reject(new Error(`agenttik exited with ${code}: ${stderr.join("")}`)));
    proc.stdout.on("data", (b) => {
      const found = /http:\/\/[\d.]+:\d+/.exec(String(b));
      if (!found) return;
      clearTimeout(timer);
      resolve(found[0]);
    });
  });

  return {
    url,
    dataDir,
    async stop() {
      proc.kill();
      await rm(dataDir, { recursive: true, force: true });
    },
  };
}

export const test = base.extend({
  agenttik: async ({}, use) => {
    const server = await startServer();
    await use(server);
    await server.stop();
  },

  /* The page opens on the app, and any error it logs fails the test — a
     silent exception in a handler is exactly the kind of break these tests
     exist to catch. A failed request is not one of those: the browser logs
     one for every 4xx, and some tests ask for a path on purpose to see the
     UI shrug it off. */
  page: async ({ page, agenttik }, use) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push("pageerror: " + e.message));
    page.on("console", (m) => {
      if (m.type() === "error" && !m.text().startsWith("Failed to load resource")) {
        errors.push(m.text());
      }
    });

    await page.goto(agenttik.url);
    await use(page);

    expect(errors, "the page logged errors").toEqual([]);
  },
});

/* The three columns, which every test needs to disambiguate its locators. */
export const sidebar = (page) => page.locator("aside").first();
export const inspector = (page) => page.locator("aside").last();

/* REPO is a real folder with real git status and a real file tree, which is
   what the project panels read. */
export const REPO = fileURLToPath(new URL("..", import.meta.url)).replace(/\/$/, "");

export async function addProject(page, path = REPO) {
  await page.getByRole("button", { name: "Add project" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const field = dialog.getByPlaceholder("~/code/myproject");
  await expect(field).toHaveValue(/.+/); // the picker prefills it with $HOME
  await field.fill(path);
  await dialog.getByRole("button", { name: "Add project", exact: true }).click();
  await expect(sidebar(page).getByText(path)).toBeVisible();
}

export async function openProject(page, path = REPO) {
  await sidebar(page).getByText(path).click();
  await expect(inspector(page).getByRole("tab", { name: "Options" })).toBeVisible();
}

/* The scriptable provider from app/internal/agent/fake, enabled above. Tests
   pick it explicitly with pickModel rather than relying on whatever
   sessionDefaults() would otherwise choose, so a turn never depends on which
   real CLIs happen to be on the machine running the suite. */
export const FAKE_MODEL = "Fake Quick";

/* Providers load in the background right after the page opens — one of them
   can be a real CLI slow to answer whether it is installed — and a click
   that lands before that finishes finds none to start a task with and fails
   outright. Retrying is simpler and more honest than guessing a fixed wait. */
export async function newTask(page) {
  const button = page.getByRole("button", { name: "New task" }).first();
  const prompt = page.getByPlaceholder("Ask the agent…");
  await expect(async () => {
    await button.click();
    await expect(prompt).toBeVisible({ timeout: 1000 });
  }).toPass({ timeout: 15_000 });
}

/* The model button opens a command palette; both it and the effort select
   beside it carry a stable title rather than a stable label, since the label
   is whatever is currently chosen — see PromptBar.vue. The title names the
   subscription as well as the model, so it is matched by prefix. */
export const modelButton = (page) => page.getByTitle(/^Choose model/);

export async function pickModel(page, label = FAKE_MODEL) {
  await modelButton(page).click();
  await page.getByRole("option", { name: label }).click();
}

/* Settings opens from the sidebar and lands on the section asked for. */
export async function openSettings(page, section = "General") {
  await sidebar(page).getByRole("button", { name: "Settings" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  if (section !== "General") {
    await page.getByRole("dialog").getByRole("button", { name: section, exact: true }).click();
  }
}

/* The default binding is Ctrl+Enter to send, plain Enter to enqueue — see
   shortcuts.js — so this reaches for the chord rather than a button, which
   only ever carries whichever of the two actions Enter currently performs. */
export async function sendPrompt(page, text) {
  const prompt = page.getByPlaceholder("Ask the agent…");
  await prompt.fill(text);
  await prompt.press("Control+Enter");
}

export { expect };
