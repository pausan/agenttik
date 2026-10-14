import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, pickModel, sendPrompt, test } from "../fixtures.js";

test("a turn's presented results are drawn under its reply", async ({ page }) => {
  const dir = await mkdtemp(join(tmpdir(), "agenttik-outputs-"));
  try {
    await mkdir(join(dir, "tasks", "r"), { recursive: true });
    await addProject(page, dir);
    await openProject(page, dir);
    await newTask(page);
    await pickModel(page);
    // The fake agent works in tasks/r, waits while the "script" writes its
    // files, then presents the report by name only.
    await sendPrompt(page, [
      '@tool Bash {"command":"cd tasks/r && python3 build.py"}',
      "@wait 1500",
      "Report: `report.html`, table `rows.csv`, chart `chart.svg`.",
    ].join("\n"));
    // Files written now fall inside the turn.
    await expect(page.getByLabel("Agent working")).toBeVisible();
    const report = '<!doctype html><h1>Conversion</h1><script>document.body.dataset.drawn = "yes"; document.querySelector("h1").onclick = () => { document.body.dataset.clicked = "yes"; };</script>';
    await writeFile(join(dir, "tasks", "r", "report.html"), report);
    await writeFile(join(dir, "tasks", "r", "rows.csv"), "week,users\n2026-10-05,42\n");
    await writeFile(join(dir, "tasks", "r", "chart.svg"), '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="green"/></svg>');
    await writeFile(join(dir, "tasks", "r", "check.png"), "not mentioned, so not a result");

    const outputs = page.getByLabel("Turn outputs");
    const card = outputs.getByRole("region", { name: "Output report.html" });
    await expect(card).toBeVisible();
    await expect(card.getByText("tasks/r", { exact: true })).toBeVisible();
    const frame = card.locator('iframe[title="tasks/r/report.html"]');
    await expect(frame).toHaveAttribute("sandbox", "allow-scripts");
    await expect(frame.contentFrame().locator("body")).toHaveAttribute("data-drawn", "yes");
    await expect(outputs.getByRole("region", { name: "Output rows.csv" }).getByRole("table")).toContainText("42");
    await expect.poll(() => outputs.getByAltText("chart.svg").evaluate((img) => img.naturalWidth)).toBe(40);
    await expect(outputs.getByAltText("check.png")).toHaveCount(0);

    await card.getByRole("button", { name: "Hide" }).click();
    await expect(frame).toHaveCount(0);
    await card.getByRole("button", { name: "Show" }).click();
    await expect(frame.contentFrame().getByRole("heading", { name: "Conversion" })).toBeVisible();

    // Commenting: a click on the page describes the element instead of
    // reaching it, and the comment lands in the task's prompt.
    await card.getByRole("button", { name: "Comment" }).click();
    await expect(card.getByRole("button", { name: "Click an element" })).toHaveAttribute("aria-pressed", "true");
    await frame.contentFrame().getByRole("heading", { name: "Conversion" }).click();
    const form = card.getByRole("form", { name: "Comment on element" });
    await expect(form).toContainText('<h1> Conversion');
    await form.getByRole("textbox", { name: "Comment" }).fill("Make the title bigger");
    await form.getByRole("textbox", { name: "Comment" }).press("Enter");
    await expect(form).toHaveCount(0);
    await expect(page.getByPlaceholder("Ask the agent…")).toHaveValue(/^On `tasks\/r\/report\.html`, at the <h1> "Conversion" \(`body > h1`\):\nMake the title bigger$/);
    expect(await frame.contentFrame().locator("body").getAttribute("data-clicked")).toBeNull();
    await card.getByRole("button", { name: "Click an element" }).click();
    await frame.contentFrame().getByRole("heading", { name: "Conversion" }).click();
    await expect(frame.contentFrame().locator("body")).toHaveAttribute("data-clicked", "yes");

    await card.getByRole("button", { name: "Open report.html in a tab" }).click();
    await expect(page.getByRole("tab", { name: "Preview", exact: true })).toHaveAttribute("aria-selected", "true");
    await expect(page.locator('iframe[title="tasks/r/report.html"]').last().contentFrame().locator("body")).toHaveAttribute("data-drawn", "yes");
    await expect(page.getByRole("button", { name: "Comment", exact: true })).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
