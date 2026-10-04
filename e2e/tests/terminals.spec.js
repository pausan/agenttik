import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
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

for (const remote of [false, true]) {
  test(`${remote ? "remote" : "local"} terminal replays and clicks leave the shell input clean`, async ({ page, agenttik }) => {
    const root = await mkdtemp(join(tmpdir(), "agenttik-terminal-replay-"));
    let client;
    let exited;
    try {
      if (remote) {
        const binary = process.env.AGENTTIK_E2E_BIN || fileURLToPath(new URL("../../bin/agenttik-web", import.meta.url));
        client = spawn(binary, ["--web", "--remote", agenttik.url]);
        exited = new Promise(resolve => client.once("exit", resolve));
        const url = await new Promise((resolve, reject) => {
          let output = "";
          const timer = setTimeout(() => reject(new Error("remote client did not start: " + output)), 10_000);
          client.once("error", error => { clearTimeout(timer); reject(error); });
          client.stdout.on("data", chunk => {
            output += chunk;
            const match = /remote client: (http:\/\/127\.0\.0\.1:\d+)/.exec(output);
            if (match) { clearTimeout(timer); resolve(match[1]); }
          });
        });
        await page.goto(url);
      }
      await addProject(page, root);
      await openProject(page, root);
      await newTask(page);
      await newTerminal(page).click();
      const inputs = [];
      page.on("request", request => {
        if (/\/api\/terminals\/[^/]+\/input$/.test(request.url())) {
          inputs.push(request.postDataBuffer().toString());
        }
      });

      // These are the queries behind the reported cursor, capability, color
      // and mode gibberish. Live output must still answer them for programs.
      await run(page, "printf '\\033[6n\\033[>c\\033]10;?\\007\\033]11;?\\007\\033[12$p\\nquery-ready\\n'");
      await expect.poll(() => inputs.join("")).toMatch(/\x1b\[12;[0-4]\$y/);
      expect(inputs.join("")).toContain("\x1b[>0;276;0c");
      expect(inputs.join("")).toContain("\x1b]10;rgb:");
      expect(inputs.join("")).toContain("\x1b]11;rgb:");
      // The probe runs at the prompt, so clear its live replies before testing
      // historical ones. A real foreground program would consume the replies.
      await page.keyboard.press("Control+u");
      await run(page, "printf 'shell-%s\\n' ready");
      await expectScreen(page, "shell-ready");

      for (const reload of [false, true]) {
        inputs.length = 0;
        if (reload) {
          await page.reload();
          await terminalTabs(page).first().click();
        } else {
          await page.getByRole("tab", { name: "New task", exact: true }).click();
          await terminalTabs(page).first().click();
        }
        await expectScreen(page, "shell-ready");
        await pane(page).locator(".xterm-screen").click();
        await page.getByRole("button", { name: "New task" }).first().focus();
        await pane(page).locator(".xterm-screen").click();
        expect(inputs).toEqual([]);
        await run(page, "printf 'clean-%s\\n' input");
        await expectScreen(page, "clean-input");
        expect(inputs.join("")).toBe("printf 'clean-%s\\n' input\r");
      }
    } finally {
      if (client) { client.kill(); await exited; }
      await rm(root, { recursive: true, force: true });
    }
  });
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

test("terminal scrollback returns to the saved line after switching tabs", async ({ page }) => {
  await addProject(page);
  await openProject(page);
  await newTask(page);
  await newTerminal(page).click();
  await run(page, "seq 1 250");
  await expectScreen(page, "250");
  const viewport = pane(page).locator(".xterm-viewport");
  await viewport.evaluate(el => { el.scrollTop = 300; });
  await expect.poll(() => viewport.evaluate(el => el.scrollTop)).toBeGreaterThan(0);
  const offset = await viewport.evaluate(el => el.scrollTop);
  const screen = await pane(page).innerText();
  await page.getByRole("tab", { name: "New task", exact: true }).click();
  await terminalTabs(page).first().click();
  await expect.poll(() => viewport.evaluate(el => el.scrollTop)).toBe(offset);
  await expect.poll(() => pane(page).innerText()).toBe(screen);
});
