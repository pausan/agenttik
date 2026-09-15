import { createServer } from "node:http";
import { expect, openSettings, test } from "../fixtures.js";

test("palette checks identity before navigating to a remote server", async ({ page }) => {
  let valid = false;
  const paths = [];
  const remote = createServer((req, res) => {
    paths.push(req.url);
    if (req.url === "/api/version") {
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ application: valid ? "agenttik" : "other", version: "test", name: "Office workstation" }));
    } else {
      res.setHeader("Content-Type", "text/html");
      res.end("<h1>Remote sign in</h1><label>Password<input type=password></label>");
    }
  });
  await new Promise((resolve) => remote.listen(0, "127.0.0.1", resolve));
  try {
    await page.keyboard.press("Control+Shift+p");
    await page.getByPlaceholder("Command Palette…").fill("Connect to remote");
    await page.getByText("Connect to remote server", { exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Connect to remote server" });
    await dialog.getByRole("textbox", { name: "Server address" }).fill(`127.0.0.1:${remote.address().port}`);
    await dialog.getByRole("button", { name: "Check server", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("did not identify itself");
    expect(paths).toEqual(["/api/version"]);
    valid = true;
    await dialog.getByRole("button", { name: "Check server", exact: true }).click();
    await expect(dialog.getByText("Office workstation", { exact: true })).toBeVisible();
    await expect(dialog.getByText("agenttik test", { exact: true })).toBeVisible();
    expect(paths).toEqual(["/api/version", "/api/version"]);
    await dialog.getByRole("button", { name: "Connect", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Remote sign in" })).toBeVisible();
    expect(paths.slice(0, 4)).toEqual(["/api/version", "/api/version", "/api/version", "/"]);
  } finally {
    await new Promise((resolve) => remote.close(resolve));
  }
});

test("remote CLI serves the remote UI without creating a local database", async ({ page, agenttik }) => {
  const { spawn } = await import("node:child_process");
  const { mkdtemp, access, rm } = await import("node:fs/promises");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const { fileURLToPath } = await import("node:url");
  const root = await mkdtemp(join(tmpdir(), "agenttik-remote-cli-"));
  const dataDir = join(root, "unused");
  const proc = spawn(fileURLToPath(new URL("../../bin/agenttik-web", import.meta.url)), [
    "--remote", agenttik.url, "--data-dir", dataDir,
  ]);
  const exited = new Promise((resolve) => proc.once("exit", resolve));
  try {
    const clientURL = await new Promise((resolve, reject) => {
      let output = "";
      const timer = setTimeout(() => reject(new Error("remote client did not start: " + output)), 10_000);
      proc.on("error", reject);
      proc.stdout.on("data", (chunk) => {
        output += chunk;
        const match = /remote client: (http:\/\/127\.0\.0\.1:\d+)/.exec(output);
        if (match) { clearTimeout(timer); resolve(match[1]); }
      });
    });
    await page.goto(clientURL);
    await page.keyboard.press("Control+Shift+p");
    await expect(page.getByPlaceholder("Command Palette…")).toBeVisible();
    await page.getByPlaceholder("Command Palette…").fill("Connect to remote");
    await page.getByText("Connect to remote server", { exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Connect to remote server" });
    await dialog.getByRole("textbox", { name: "Server address" }).fill(agenttik.url);
    await dialog.getByRole("button", { name: "Check server", exact: true }).click();
    await expect(dialog.getByText(/agenttik-/)).toBeVisible();
    await Promise.all([
      page.waitForNavigation(),
      dialog.getByRole("button", { name: "Connect", exact: true }).click(),
    ]);
    expect(page.url()).toBe(clientURL + "/");
    const response = await page.request.get(clientURL + "/api/version");
    const info = await response.json();
    expect(info.application).toBe("agenttik");
    expect(info.version).toBeTruthy();
    expect(response.headers()["x-agenttik-remote"]).toBe(agenttik.url);
    await expect(access(dataDir)).rejects.toThrow();
  } finally {
    proc.kill();
    await exited;
    await rm(root, { recursive: true, force: true });
  }
});

test("server names validate and persist in settings and the sidebar", async ({ page }) => {
  await openSettings(page, "Server");
  const input = page.getByRole("textbox", { name: "Server name", exact: true });
  await expect(input).toHaveValue(/^agenttik-[a-f0-9]{8}$/);
  const form = page.locator("form").filter({ has: input });
  for (const name of ["", "  ab  "]) {
    await input.fill(name);
    await form.getByRole("button", { name: "Save", exact: true }).click();
    await expect(form.getByText("Server name must contain at least 3 characters.")).toBeVisible();
  }
  await input.fill("  Office workstation  ");
  await form.getByRole("button", { name: "Save", exact: true }).click();
  await expect(input).toHaveValue("Office workstation");
  await page.reload();
  await expect(page.locator("aside").getByText("Office workstation", { exact: true })).toBeVisible();
  await openSettings(page, "Server");
  await expect(input).toHaveValue("Office workstation");
});
