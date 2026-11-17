import { execFileSync } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { addProject, expect, newTask, openProject, pickModel, sendPrompt, sidebar, test } from "../fixtures.js";

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
    // In full screen only the player is drawn, so the menu opens inside it.
    await page.getByRole("button", { name: "Full screen", exact: true }).click();
    await expect(page.getByRole("button", { name: "Exit full screen", exact: true })).toBeVisible();
    await page.getByRole("combobox", { name: "Playback speed" }).click();
    await page.getByRole("option", { name: "1.5×", exact: true }).click();
    expect((await state()).rate).toBe(1.5);
    await page.getByRole("button", { name: "Exit full screen", exact: true }).click();
    await expect(page.getByRole("button", { name: "Full screen", exact: true })).toBeVisible();
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

test("audio files play in the same player, without a picture or full screen", async ({ page }) => {
  test.skip(!hasFFmpeg(), "needs ffmpeg and ffprobe");
  const dir = await mkdtemp(join(tmpdir(), "agenttik-audio-"));
  try {
    const tone = (path, codec) => execFileSync("ffmpeg", ["-v", "error", "-f", "lavfi", "-i", "sine=duration=3", "-c:a", codec, path]);
    tone(join(dir, "song.mp3"), "mp3");
    tone(join(dir, "song.wma"), "wmav2");
    await addProject(page, dir);
    await openProject(page, dir);
    await sidebar(page).getByRole("tab", { name: "Tree" }).click();

    const media = page.locator("video");
    const time = page.getByLabel("Playback time", { exact: true });
    await sidebar(page).getByRole("button", { name: "song.mp3", exact: true }).click();
    await expect(time).toHaveText("0:00 / 0:03");
    await expect(page.getByLabel("Audio player", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Full screen", exact: true })).toHaveCount(0);
    await expect(page.getByText("converted", { exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "Play", exact: true }).click();
    await expect.poll(() => media.evaluate((el) => el.currentTime)).toBeGreaterThan(0.2);
    await page.getByRole("button", { name: "Pause", exact: true }).click();

    // What the browser cannot decode is converted to sound alone.
    await sidebar(page).getByRole("button", { name: "song.wma", exact: true }).click();
    await expect(page.getByText("converted", { exact: true })).toBeVisible();
    await expect(time).toHaveText("0:00 / 0:03");
    await page.getByRole("button", { name: "Play", exact: true }).click();
    await expect.poll(() => media.evaluate((el) => el.currentTime)).toBeGreaterThan(0.2);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("a sound file named in a reply gets a player in the conversation, if it exists", async ({ page }) => {
  test.skip(!hasFFmpeg(), "needs ffmpeg and ffprobe");
  const dir = await mkdtemp(join(tmpdir(), "agenttik-audio-ref-"));
  try {
    execFileSync("ffmpeg", ["-v", "error", "-f", "lavfi", "-i", "sine=duration=3", "-c:a", "mp3", join(dir, "song.mp3")]);
    await addProject(page, dir);
    await openProject(page, dir);
    await newTask(page);
    await pickModel(page);
    await sendPrompt(page, "Made `song.mp3`, not `missing.mp3`.");

    const player = page.getByLabel("Audio song.mp3", { exact: true });
    await expect(player).toBeVisible();
    await expect(page.getByLabel("Audio missing.mp3", { exact: true })).toHaveCount(0);
    const audio = player.locator("audio");
    await expect.poll(() => audio.evaluate((el) => el.duration)).toBeCloseTo(3, 0);
    await audio.evaluate((el) => el.play());
    await expect.poll(() => audio.evaluate((el) => el.currentTime)).toBeGreaterThan(0.2);
    await audio.evaluate((el) => el.pause());

    await page.evaluate(() => {
      window.openedLinks = [];
      window.open = (url) => { window.openedLinks.push(String(url)); return null; };
    });
    await player.getByRole("button", { name: "Open in browser", exact: true }).click();
    await expect.poll(() => page.evaluate(() => window.openedLinks)).toEqual([expect.stringMatching(/\/api\/projects\/\d+\/video\?path=song\.mp3/)]);

    await player.getByRole("button", { name: "Open in tab", exact: true }).click();
    await expect(page.getByRole("tab", { name: "song.mp3", exact: true })).toBeVisible();
    await expect(page.getByLabel("Audio player", { exact: true })).toBeVisible();
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
