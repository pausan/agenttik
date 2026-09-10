import { spawn } from "node:child_process";
import { access, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test as base } from "@playwright/test";

const BIN = fileURLToPath(new URL("../bin/agenttik-web", import.meta.url));

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
  const proc = spawn(BIN, ["--web", "--addr", "127.0.0.1:0", "--data-dir", dataDir]);
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
  const field = page.getByPlaceholder("~/code/myproject");
  await expect(field).toHaveValue(/.+/); // the picker prefills it with $HOME
  await field.fill(path);
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(sidebar(page).getByText(path)).toBeVisible();
}

export async function openProject(page, path = REPO) {
  await sidebar(page).getByText(path).click();
  await expect(inspector(page).getByRole("tab", { name: "Options" })).toBeVisible();
}

export { expect };
