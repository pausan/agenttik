import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test, REPO } from "../fixtures.js";

const binary = fileURLToPath(new URL("../../bin/agenttik-web", import.meta.url));

test("private instances coexist, keep browser state ephemeral, and clean up on SIGTERM", async ({ page, agenttik }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-private-test-"));
  const children = [];
  try {
    for (let i = 0; i < 2; i++) {
      const proc = spawn(binary, ["--web", "--private", "--data-dir", agenttik.dataDir], {
        env: { ...process.env, TMPDIR: root, AGENTTIK_FAKE_PROVIDER: "1" },
      });
      const exited = once(proc, "exit");
      children.push({ proc, exited });
      const url = await new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("Private server did not start")), 10000);
        proc.on("error", reject);
        proc.stdout.on("data", data => {
          const match = /http:\/\/[\d.]+:\d+/.exec(String(data));
          if (match) { clearTimeout(timer); resolve(match[0]); }
        });
      });
      children[i].url = url;
    }
    const dirs = (await readdir(root)).filter(name => name.startsWith("agenttik-private-"));
    expect(dirs).toHaveLength(2);
    await page.request.post(`${children[0].url}/api/projects`, { data: { path: REPO, name: "Private project" } });
    expect(await (await page.request.get(`${children[1].url}/api/projects`)).json()).toEqual([]);
    expect(await (await page.request.get(`${agenttik.url}/api/projects`)).json()).toEqual([]);
    await page.goto(children[0].url);
    await expect(page.getByText("Private", { exact: true })).toBeVisible();
    await expect(page.getByText("Private project", { exact: true }).first()).toBeVisible();
    await page.reload();
    expect(await page.evaluate(() => Object.keys(localStorage).filter(key => key.startsWith("agenttik.")))).toEqual([]);
    for (const child of children) { child.proc.kill("SIGTERM"); await child.exited; }
    expect((await readdir(root)).filter(name => name.startsWith("agenttik-private-"))).toEqual([]);
    expect((await page.request.get(`${agenttik.url}/api/projects`)).ok()).toBe(true);
  } finally {
    for (const child of children) { if (child.proc.exitCode === null && child.proc.signalCode === null) { child.proc.kill("SIGTERM"); await child.exited; } }
    await rm(root, { recursive: true, force: true });
  }
});
