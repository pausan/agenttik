<script>
import { ref } from "vue";

// Volume and mute carry over from one video to the next, as in any player.
const volume = ref(1);
const muted = ref(false);
</script>

<script setup>
/* A video, played in place with its own controls: play, seek, speed, volume,
   loop and full screen.

   The file streams by range, so a seek reads only what it lands on. What the
   browser cannot decode — an AVI, an MPEG, most MKVs outside Chromium — is
   converted by ffmpeg on the server while it plays. Such a stream has no
   length of its own and cannot be seeked by range, so the length comes from
   ffprobe and a seek past what has arrived asks for a new one from there.
   offset is where the current stream starts in the file. */
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

import { api } from "../api";
import { videoURL } from "../store";
import { formatTime, inRanges, mediaBase, SPEEDS, stepSpeed } from "../video";

const props = defineProps({ tab: { type: Object, required: true } });
const emit = defineEmits(["dimensions"]);

const box = ref(null);
const video = ref(null);
const convert = ref(false);
const offset = ref(0);
const length = ref(NaN);
const status = ref("loading");
const problem = ref("");
const playing = ref(false);
const time = ref(0);
const rate = ref(1);
const loop = ref(false);
const scrub = ref(null);
const fullscreen = ref(false);
// Play once the next stream has loaded: a seek that restarts ffmpeg keeps
// playing if it was.
let resume = false;

const base = ref(null);
mediaBase().then((b) => { base.value = b; });
const src = computed(() => base.value === null ? undefined : base.value + videoURL(props.tab, convert.value, offset.value));
const known = computed(() => Number.isFinite(length.value) && length.value > 0);
const shown = computed(() => scrub.value ?? time.value);
const speeds = SPEEDS.map((s) => ({ label: `${s}×`, value: s }));
const volumeIcon = computed(() =>
  muted.value || volume.value === 0 ? "i-lucide-volume-x" : volume.value < 0.5 ? "i-lucide-volume-1" : "i-lucide-volume-2",
);

watch(
  () => [props.tab.projectID, props.tab.path],
  () => {
    convert.value = false;
    offset.value = 0;
    length.value = NaN;
    status.value = "loading";
    problem.value = "";
    playing.value = false;
    time.value = 0;
    // The element is reused, and a new source starts at its default rate.
    setRate(1);
    loop.value = false;
    scrub.value = null;
    resume = false;
  },
);

function onMetadata() {
  const el = video.value;
  el.defaultPlaybackRate = el.playbackRate = rate.value;
  el.volume = volume.value;
  el.muted = muted.value;
  if (el.videoWidth) emit("dimensions", { width: el.videoWidth, height: el.videoHeight });
  if (!convert.value) {
    length.value = el.duration;
    // A container the browser opens but whose picture it cannot decode
    // plays as sound alone, with no error to say so.
    if (!el.videoWidth) return fallback(true);
  }
  status.value = "ready";
  if (resume) {
    resume = false;
    el.play().catch(() => {});
  }
}

function onError() {
  const code = video.value?.error?.code;
  // 3 is a decode failure, 4 a format this browser does not play.
  if (!convert.value && (code === 3 || code === 4)) return fallback(false);
  status.value = "error";
  problem.value = "Cannot play this video.";
}

async function fallback(onlyWithPicture) {
  const tab = props.tab;
  status.value = "loading";
  let info;
  try {
    info = await api("GET", `/api/projects/${tab.projectID}/video-info?path=${encodeURIComponent(tab.path)}`);
  } catch (e) {
    if (tab.path !== props.tab.path) return;
    status.value = "error";
    problem.value = e.message;
    return;
  }
  if (tab.path !== props.tab.path) return;
  if (onlyWithPicture && !info.video) {
    status.value = "ready";
    return;
  }
  if (!info.transcode) {
    status.value = "error";
    problem.value = "This browser cannot play this video, and converting it needs ffmpeg, which is not installed.";
    return;
  }
  length.value = info.duration || NaN;
  resume = playing.value;
  convert.value = true;
}

function onTime() {
  time.value = offset.value + (video.value?.currentTime || 0);
}

function onEnded() {
  playing.value = false;
  if (convert.value && loop.value) seek(0, true);
}

function seek(t, play = playing.value) {
  const el = video.value;
  if (!el) return;
  t = Math.max(0, known.value ? Math.min(t, length.value) : t);
  time.value = t;
  if (!convert.value) {
    el.currentTime = t;
    return;
  }
  if (inRanges(el.buffered, t - offset.value)) {
    el.currentTime = t - offset.value;
    if (play && el.paused) el.play().catch(() => {});
    return;
  }
  resume = play;
  status.value = "loading";
  offset.value = t;
}

function toggle() {
  const el = video.value;
  if (!el || status.value === "error") return;
  if (!el.paused) return el.pause();
  // A converted stream that ended begins where it was asked for, not at the
  // start of the file.
  if (el.ended && convert.value && offset.value > 0) return seek(0, true);
  el.play().catch(() => {});
}

// A converted seek waits for the release: each one starts ffmpeg again.
function onScrub(e) {
  const t = Number(e.target.value);
  if (convert.value) scrub.value = t;
  else seek(t);
}

function onScrubEnd(e) {
  scrub.value = null;
  seek(Number(e.target.value));
}

function setRate(r) {
  rate.value = r;
  if (video.value) video.value.defaultPlaybackRate = video.value.playbackRate = r;
}

function setVolume(v) {
  volume.value = Math.min(1, Math.max(0, Math.round(v * 100) / 100));
  muted.value = volume.value === 0;
  if (video.value) {
    video.value.volume = volume.value;
    video.value.muted = muted.value;
  }
}

function toggleMute() {
  if (muted.value && volume.value === 0) return setVolume(1);
  muted.value = !muted.value;
  if (video.value) video.value.muted = muted.value;
}

function toggleFullscreen() {
  if (document.fullscreenElement) document.exitFullscreen?.();
  else box.value?.requestFullscreen?.().catch(() => {});
}

const onFullscreen = () => { fullscreen.value = document.fullscreenElement === box.value; };
onMounted(() => document.addEventListener("fullscreenchange", onFullscreen));
onUnmounted(() => document.removeEventListener("fullscreenchange", onFullscreen));

/* Keys act while the player itself has focus, so they never reach past it
   into the app's own shortcuts, nor take arrows from the sliders. */
function onKey(e) {
  if (e.target !== box.value && e.target !== video.value) return;
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  const actions = {
    " ": toggle,
    k: toggle,
    ArrowLeft: () => seek(time.value - 5),
    ArrowRight: () => seek(time.value + 5),
    j: () => seek(time.value - 10),
    l: () => seek(time.value + 10),
    Home: () => seek(0),
    ArrowUp: () => setVolume(volume.value + 0.1),
    ArrowDown: () => setVolume(volume.value - 0.1),
    m: toggleMute,
    f: toggleFullscreen,
    "<": () => setRate(stepSpeed(rate.value, -1)),
    ">": () => setRate(stepSpeed(rate.value, 1)),
  };
  const action = actions[e.key];
  if (!action) return;
  e.preventDefault();
  e.stopPropagation();
  action();
}
</script>

<template>
  <div
    ref="box"
    tabindex="0"
    class="flex h-full min-h-0 flex-col bg-black outline-none"
    aria-label="Video player"
    @keydown="onKey"
  >
    <div class="relative flex min-h-0 flex-1 items-center justify-center">
      <video
        ref="video"
        class="h-full w-full object-contain"
        :src="src"
        :loop="loop && !convert"
        preload="metadata"
        playsinline
        @loadedmetadata="onMetadata"
        @timeupdate="onTime"
        @play="playing = true"
        @pause="playing = false"
        @ended="onEnded"
        @error="onError"
        @click="toggle"
        @dblclick="toggleFullscreen"
      ></video>
      <p v-if="status === 'loading'" role="status" class="absolute text-sm text-neutral-300">
        {{ convert ? "Converting video…" : "Loading video…" }}
      </p>
      <p v-else-if="status === 'error'" role="alert" class="absolute max-w-md px-5 text-center text-sm text-warning">
        {{ problem }}
      </p>
    </div>

    <div class="flex shrink-0 flex-wrap items-center gap-2 border-t border-default bg-default px-3 py-1.5 text-xs text-dimmed">
      <UButton
        :icon="playing ? 'i-lucide-pause' : 'i-lucide-play'"
        size="xs"
        color="neutral"
        variant="ghost"
        :aria-label="playing ? 'Pause' : 'Play'"
        :title="playing ? 'Pause (Space)' : 'Play (Space)'"
        :disabled="status === 'error'"
        @click="toggle"
      />
      <span class="shrink-0 font-mono tabular-nums" aria-label="Playback time">
        {{ formatTime(shown) }} / {{ formatTime(length) }}
      </span>
      <input
        type="range"
        class="min-w-24 flex-1 accent-(--ui-primary)"
        aria-label="Seek"
        min="0"
        :max="known ? length : 0"
        step="0.01"
        :value="shown"
        :disabled="!known || status === 'error'"
        @input="onScrub"
        @change="onScrubEnd"
      />
      <USelect
        :model-value="rate"
        :items="speeds"
        size="xs"
        class="w-20"
        aria-label="Playback speed"
        title="Playback speed (< and >)"
        @update:model-value="setRate(Number($event))"
      />
      <UButton
        :icon="volumeIcon"
        size="xs"
        color="neutral"
        variant="ghost"
        :aria-label="muted ? 'Unmute' : 'Mute'"
        :title="muted ? 'Unmute (M)' : 'Mute (M)'"
        @click="toggleMute"
      />
      <input
        type="range"
        class="w-20 accent-(--ui-primary)"
        aria-label="Volume"
        min="0"
        max="1"
        step="0.05"
        :value="muted ? 0 : volume"
        @input="setVolume(Number($event.target.value))"
      />
      <UButton
        icon="i-lucide-repeat"
        size="xs"
        color="neutral"
        :variant="loop ? 'soft' : 'ghost'"
        :aria-pressed="loop"
        aria-label="Loop"
        title="Loop"
        @click="loop = !loop"
      />
      <UButton
        :icon="fullscreen ? 'i-lucide-minimize-2' : 'i-lucide-maximize-2'"
        size="xs"
        color="neutral"
        variant="ghost"
        :aria-label="fullscreen ? 'Exit full screen' : 'Full screen'"
        :title="fullscreen ? 'Exit full screen (F)' : 'Full screen (F)'"
        @click="toggleFullscreen"
      />
      <span v-if="convert" class="shrink-0" title="This browser cannot decode the file, so ffmpeg converts it while it plays">
        converted
      </span>
    </div>
  </div>
</template>
