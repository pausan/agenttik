import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, sidebar, test } from "../fixtures.js";

/* Terminal tabs — see specs/073-terminals.md. The fixture pins SHELL to
   /bin/sh, so the tabs here are named "sh 1", "sh 2" and the screen is one
   this suite can read on any machine. Nothing below reads a prompt: each
   test types a command and looks for what it printed. */

/* The button at the other end of the prompt's line of chord hints. */
const newTerminal = (page) => page.getByRole("button", { name: "Open a terminal in the project folder" });
const terminalTabs = (page) => page.getByRole("tab", { name: /^sh \d+$/ });
const pane = (page) => page.locator(".terminal-pane");

async function run(page, line) {
  await pane(page).locator(".xterm-screen").click();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
}

/* xterm draws the screen as rows of spans, so it is read whole rather than a
   line at a time. A shell takes a moment to answer, and the reply arrives in
   pieces, so this waits for the text rather than asserting on the first look. */
async function expectScreen(page, text) {
  await expect(async () => {
    expect(await pane(page).innerText()).toContain(text);
  }).toPass({ timeout: 15_000 });
}

test("a terminal runs in the project folder, and closing it ends the shell", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-"));
  try {
    await writeFile(join(root, "in-this-project.txt"), "hello");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);

    await newTerminal(page).click();
    await expect(terminalTabs(page)).toHaveCount(1);
    await expect(pane(page)).toBeVisible();

    // The shell starts where the agent works, not where the app was launched.
    await run(page, "ls");
    await expectScreen(page, "in-this-project.txt");

    // Closing the tab is closing the terminal.
    await terminalTabs(page).first().getByRole("button", { name: /^Close/ }).click();
    await expect(pane(page)).toHaveCount(0);
    await expect(terminalTabs(page)).toHaveCount(0);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("terminals belong to the project: another project hides them, and coming back brings them back in order", async ({ page }) => {
  const one = await mkdtemp(join(tmpdir(), "agenttik-terminal-one-"));
  const two = await mkdtemp(join(tmpdir(), "agenttik-terminal-two-"));
  try {
    await addProject(page, one);
    await addProject(page, two);
    await openProject(page, one);
    await newTask(page);

    // Two terminals in the first project, each marked so they can be told
    // apart on the way back.
    await newTerminal(page).click();
    await run(page, "echo first-terminal");
    await expectScreen(page, "first-terminal");

    await page.getByRole("tab", { name: "New task" }).first().click();
    await newTerminal(page).click();
    await run(page, "echo second-terminal");
    await expectScreen(page, "second-terminal");
    await expect(terminalTabs(page)).toHaveCount(2);

    // The other project's strip has none of them.
    await sidebar(page).getByText(two).click();
    await expect(terminalTabs(page)).toHaveCount(0);
    await expect(pane(page)).toHaveCount(0);

    // Coming back shows the same two, in the order they were opened, still
    // holding what was typed into them.
    await sidebar(page).getByText(one).click();
    await expect(terminalTabs(page)).toHaveCount(2);
    await expect(terminalTabs(page).nth(0)).toHaveAttribute("aria-label", "sh 1");
    await expect(terminalTabs(page).nth(1)).toHaveAttribute("aria-label", "sh 2");

    await terminalTabs(page).nth(0).click();
    await expectScreen(page, "first-terminal");
    await terminalTabs(page).nth(1).click();
    await expectScreen(page, "second-terminal");
  } finally {
    await rm(one, { recursive: true, force: true });
    await rm(two, { recursive: true, force: true });
  }
});

test("a second task leaves the terminals where they were", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-tasks-"));
  try {
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await newTerminal(page).click();
    await run(page, "echo still-here");
    await expectScreen(page, "still-here");

    // A terminal belongs to the project, so which task sits in the context
    // slot beside it is not its business.
    await newTask(page);
    await expect(terminalTabs(page)).toHaveCount(1);
    await terminalTabs(page).first().click();
    await expectScreen(page, "still-here");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("a shell that exits takes its tab with it", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-exit-"));
  try {
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await newTerminal(page).click();
    await expect(pane(page)).toBeVisible();

    await run(page, "exit");
    await expect(terminalTabs(page)).toHaveCount(0);
    await expect(pane(page)).toHaveCount(0);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("Ctrl+Alt+T opens a terminal, from the prompt and from a terminal alike", async ({ page }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-chord-"));
  try {
    await writeFile(join(root, "in-this-project.txt"), "hello");
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);

    // From the prompt box, where the chord's own button sits.
    await page.keyboard.press("Control+Alt+t");
    await expect(terminalTabs(page)).toHaveCount(1);
    await run(page, "ls");
    await expectScreen(page, "in-this-project.txt");

    // And from inside the terminal it just opened, which is the likeliest
    // place to want a second one. xterm.js would otherwise turn the chord
    // into an escape sequence and stop the event before the window saw it.
    await page.keyboard.press("Control+Alt+t");
    await expect(terminalTabs(page)).toHaveCount(2);
    await expect(terminalTabs(page).nth(1)).toHaveAttribute("aria-label", "sh 2");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("terminal clipboard menu and shortcuts copy selection and paste into the shell", async ({ page, context }) => {
  const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-clipboard-"));
  try {
    await addProject(page, root);
    await openProject(page, root);
    await newTask(page);
    await newTerminal(page).click();
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    const screen = pane(page).locator(".xterm-screen");
    await screen.click({ button: "right" });
    await expect(page.getByRole("menuitem", { name: "Copy", exact: true })).toHaveAttribute("data-disabled", "");
    await page.keyboard.press("Escape");

    await page.evaluate(() => navigator.clipboard.writeText("printf 'clipboard-%s\\n' menu"));
    await screen.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Paste", exact: true }).click();
    await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(pane(page).locator("textarea")).toBeFocused();
    await page.keyboard.press("Enter");
    await expectScreen(page, "clipboard-menu");

    await pane(page).locator(".xterm-rows").getByText("clipboard-menu", { exact: true }).dblclick();
    await screen.click({ button: "right" });
    await page.getByRole("menuitem", { name: "Copy", exact: true }).click();
    await expect(page.getByRole("menu")).toHaveCount(0);
    await expect(pane(page).locator("textarea")).toBeFocused();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("clipboard-menu");

    await page.evaluate(() => navigator.clipboard.writeText("printf 'clipboard-%s\\n' shortcut"));
    await page.keyboard.press("Control+Shift+V");
    await expectScreen(page, "shortcut");
    await page.keyboard.press("Enter");
    await expectScreen(page, "clipboard-shortcut");
    await pane(page).locator(".xterm-rows").getByText("clipboard-shortcut", { exact: true }).dblclick();
    await page.keyboard.press("Control+Shift+C");
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("clipboard-shortcut");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
