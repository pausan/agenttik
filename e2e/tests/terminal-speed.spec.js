import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, test } from "../fixtures.js";

const pane = (page) => page.locator(".terminal-pane");

// PERF_BASELINE measures the original write queue without the new time budgets.
test("large terminal listings finish within the rendering budget", async ({ page }) => {
  test.setTimeout(120_000);
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-speed-"));
  try {
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await page.getByRole("button", { name: "Open a terminal in the project folder" }).click();
    await pane(page).locator(".xterm-screen").click();
    const sizes = process.env.PERF_BASELINE ? [20000, 200000] : [20000, 200000, 1000000];
    for (const n of sizes) {
      const budget = n === 1000000 ? 15_000 : 5_000;
      await page.keyboard.type(`clear; seq 1 ${n}; printf 'bench-%s\\n' ${n}`);
      const start = Date.now();
      await page.keyboard.press("Enter");
      await expect(async () => {
        expect(await pane(page).innerText()).toContain(`bench-${n}`);
      }).toPass({ timeout: process.env.PERF_BASELINE ? 100_000 : budget, intervals: [20] });
      const elapsed = Date.now() - start;
      console.log(`seq ${n}: ${elapsed}ms`);
      // The marker is assembled by printf, so the echoed command cannot pass.
      await expect(pane(page).locator(".xterm-rows").getByText(String(n), { exact: true })).toBeVisible();
      if (!process.env.PERF_BASELINE) {
        expect(elapsed, `rendering ${n} lines`).toBeLessThan(budget);
      }
    }
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("a busy terminal still accepts Ctrl+C and the next command", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-interrupt-"));
  try {
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await page.getByRole("button", { name: "Open a terminal in the project folder" }).click();
    await pane(page).locator(".xterm-screen").click();
    await page.keyboard.type("yes busy-output");
    await page.keyboard.press("Enter");
    // Wait for actual output before interrupting, rather than the command echo.
    await expect(pane(page).locator(".xterm-rows").getByText("busy-output", { exact: true }).first()).toBeVisible();
    const start = Date.now();
    await page.keyboard.press("Control+c");
    await page.keyboard.type("printf 'interrupt-%s\\n' done");
    await page.keyboard.press("Enter");
    await expect.poll(() => pane(page).innerText(), { timeout: 5_000, intervals: [20] }).toContain("interrupt-done");
    expect(Date.now() - start, "interrupt and next command").toBeLessThan(5_000);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
