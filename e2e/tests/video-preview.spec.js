import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, openProject, sidebar, test } from "../fixtures.js";

function hasFFmpeg() {
  try {
    execFileSync("ffmpeg", ["-version"], { stdio: "ignore" });
    execFileSync("ffprobe", ["-version"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

// Four seconds of test pattern and tone, in the given codecs.
function clip(path, video, audio) {
  execFileSync("ffmpeg", ["-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x120:rate=25:duration=4",
    "-f", "lavfi", "-i", "sine=duration=4", "-c:v", video, "-c:a", audio, path]);
}

test("videos play in place with speed, volume, loop and seeking, converted when the browser cannot decode them", async ({ page }) => {
  test.skip(!hasFFmpeg(), "needs ffmpeg and ffprobe");
  const dir = await mkdtemp(join(tmpdir(), "agenttik-videos-"));
  try {
    clip(join(dir, "native.webm"), "libvpx", "libopus");
    clip(join(dir, "legacy.avi"), "mpeg4", "mp3");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();

    const requests = [];
    page.on("request", (req) => requests.push(req.url()));
    const video = page.locator("video");
    const time = page.getByLabel("Playback time", { exact: true });
    const state = () => video.evaluate((el) => ({ rate: el.playbackRate, volume: el.volume, muted: el.muted, loop: el.loop, paused: el.paused, time: el.currentTime }));

    // A format the browser plays streams as it is, in ranges.
    await sidebar(page).getByRole("button", { name: "native.webm", exact: true }).click();
    await expect(time).toHaveText("0:00 / 0:04");
    await expect(page.getByRole("tab", { name: "Edit", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Save", exact: true })).toHaveCount(0);
    await expect(page.getByLabel("File information", { exact: true })).toHaveText(/, 160x120\)$/);
    await expect(page.getByText("converted", { exact: true })).toHaveCount(0);
    expect(requests.some((url) => /\/file\?.*native\.webm/.test(url))).toBe(false);

    await page.getByRole("button", { name: "Play", exact: true }).click();
    await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible();
    await expect.poll(async () => (await state()).time).toBeGreaterThan(0.2);
    await page.getByRole("button", { name: "Pause", exact: true }).click();
    expect((await state()).paused).toBe(true);

    await page.getByRole("combobox", { name: "Playback speed" }).click();
    await page.getByRole("option", { name: "2×", exact: true }).click();
    expect((await state()).rate).toBe(2);
    await page.getByRole("slider", { name: "Volume", exact: true }).fill("0.35");
    expect((await state()).volume).toBeCloseTo(0.35);
    await page.getByRole("button", { name: "Mute", exact: true }).click();
    expect((await state()).muted).toBe(true);
    await page.getByRole("button", { name: "Unmute", exact: true }).click();
    await page.getByRole("button", { name: "Loop", exact: true }).click();
    expect((await state()).loop).toBe(true);
    await page.getByRole("slider", { name: "Seek", exact: true }).fill("3");
    await expect.poll(async () => (await state()).time).toBeCloseTo(3, 1);
    await expect(time).toHaveText("0:03 / 0:04");

    // Keys belong to the focused player.
    await page.getByLabel("Video player", { exact: true }).focus();
    await page.keyboard.press("Home");
    await expect(time).toHaveText("0:00 / 0:04");
    await page.keyboard.press(">");
    expect((await state()).rate).toBe(3);
    await page.keyboard.press("m");
    expect((await state()).muted).toBe(true);
    await page.keyboard.press("m");
    await page.keyboard.press(" ");
    await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible();
    await page.keyboard.press(" ");

    // An AVI is converted by ffmpeg, its length read by ffprobe, and a seek
    // the stream cannot make starts the conversion again from there.
    await sidebar(page).getByRole("button", { name: "legacy.avi", exact: true }).click();
    await expect(page.getByText("converted", { exact: true })).toBeVisible();
    await expect(time).toHaveText("0:00 / 0:04");
    expect(requests.some((url) => /video\?path=legacy\.avi&transcode=1&start=0\.000/.test(url))).toBe(true);
    // Speed resets with the file; volume carries over.
    expect((await state()).rate).toBe(1);
    expect((await state()).volume).toBeCloseTo(0.35);
    await page.getByRole("button", { name: "Play", exact: true }).click();
    await expect.poll(async () => (await state()).time).toBeGreaterThan(0.2);
    await page.getByRole("slider", { name: "Seek", exact: true }).fill("2.5");
    await expect(time).toHaveText(/^0:0[23] \/ 0:04$/);
    await expect(page.getByRole("button", { name: "Pause", exact: true })).toBeVisible();
    await expect(page.getByRole("alert")).toHaveCount(0);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
