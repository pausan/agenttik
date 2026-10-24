import { computed, reactive } from "vue";
import { api } from "./api";
import { S, fail } from "./store";
import { promptImages, promptText, withImages } from "./prompt-images";
import { createDictation, microphone } from "./speech-recorder";

export const dictation = reactive({ phase: "idle" });
export const dictationLabel = computed(() => ({
  idle: "Dictate a prompt", permission: "Opening microphone…", preparing: "Preparing speech recognition…",
  downloading: "Downloading speech model…", recording: "Stop recording", transcribing: "Transcribing…",
})[dictation.phase]);

const controller = createDictation({
  state: dictation, request: api, capture: microphone,
  deliver(owner, text) {
    const existing = promptText(owner.draft);
    owner.draft = withImages([existing, text].filter(Boolean).join(existing.endsWith("\n") ? "" : "\n"), promptImages(owner.draft));
    if (S.owner === owner) S.promptFocus++;
  },
  onError: fail,
  onWarning: (message) => fail(new Error(message)),
});
export function toggleDictation() { return controller.toggle(S.owner); }
export function cancelDictation() { controller.cancel(); }
globalThis.addEventListener?.("pagehide", cancelDictation);
