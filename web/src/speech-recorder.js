// Capture on the client; send one bounded PCM16 recording when the user stops.
// Averaging source samples before downsampling avoids simply dropping samples.
export function pcm16(chunks, sampleRate) {
  const length = chunks.reduce((n, chunk) => n + chunk.length, 0);
  const samples = new Float32Array(length);
  let offset = 0;
  for (const chunk of chunks) { samples.set(chunk, offset); offset += chunk.length; }
  const ratio = sampleRate / 16000;
  const count = Math.min(16000 * 120, Math.floor(length / ratio));
  const bytes = new Uint8Array(count * 2);
  const view = new DataView(bytes.buffer);
  for (let i = 0; i < count; i++) {
    const start = i * ratio, end = Math.min(length, (i + 1) * ratio);
    let sum = 0;
    for (let j = Math.floor(start); j < Math.ceil(end); j++) {
      sum += samples[j] * (Math.min(j + 1, end) - Math.max(j, start));
    }
    const value = Math.max(-1, Math.min(1, sum / (end - start)));
    view.setInt16(i * 2, Math.round(value * (value < 0 ? 32768 : 32767)), true);
  }
  return bytes;
}

// Root mean square of samples in [-1, 1].
export function rms(samples) {
  let sum = 0;
  for (let i = 0; i < samples.length; i++) sum += samples[i] * samples[i];
  return samples.length ? Math.sqrt(sum / samples.length) : 0;
}

// Map RMS to 0..1 on a decibel scale: -55 dBFS (room noise) to -10 dBFS (loud speech).
export function loudness(value) {
  return value > 0 ? Math.max(0, Math.min(1, (20 * Math.log10(value) + 55) / 45)) : 0;
}

export function base64PCM(bytes) {
  let text = "";
  for (let i = 0; i < bytes.length; i += 8192) text += String.fromCharCode(...bytes.subarray(i, i + 8192));
  return btoa(text);
}

// The mobile app's WebView hides getUserMedia from plain-HTTP servers, so the
// app records natively and returns the same 16 kHz PCM16 base64. iOS replies
// through a promise; Android replies by message, matched by id. While recording,
// "level" is polled for the RMS of the latest audio so the UI can show a meter.
export function nativeMicrophone(scope = globalThis) {
  const ios = scope.webkit?.messageHandlers?.agenttikMicrophone;
  const android = scope.agenttikMicrophone;
  let send;
  if (ios) send = (command) => ios.postMessage({ command });
  else if (android?.postMessage) {
    const pending = new Map();
    let next = 0;
    android.onmessage = (event) => {
      const reply = JSON.parse(event.data), settle = pending.get(reply.id);
      pending.delete(reply.id);
      if (reply.error) settle?.reject(new Error(reply.error)); else settle?.resolve(reply.data ?? "");
    };
    send = (command) => new Promise((resolve, reject) => {
      const id = ++next;
      pending.set(id, { resolve, reject });
      android.postMessage(JSON.stringify({ id, command }));
    });
  } else return null;
  return async () => {
    await send("permission");
    let timer, poll, polling = false;
    const end = () => { clearTimeout(timer); clearInterval(poll); };
    return {
      start(onLimit, onLevel) {
        void send("start").catch(() => {});
        timer = setTimeout(onLimit, 120000);
        if (onLevel) poll = setInterval(() => {
          if (polling) return;
          polling = true;
          send("level").then((value) => { if (polling) onLevel(loudness(Number(value) || 0)); })
            .catch(() => {}).finally(() => { polling = false; });
        }, 100);
      },
      stop() { end(); polling = false; return send("stop"); },
      cancel() { end(); polling = false; void send("cancel").catch(() => {}); },
    };
  };
}

let native;
export async function microphone() {
  const AudioContext = globalThis.AudioContext || globalThis.webkitAudioContext;
  if (!navigator.mediaDevices?.getUserMedia || !AudioContext) {
    native ??= nativeMicrophone();
    if (native) return native();
    throw new Error("Microphone access is unavailable. Use the desktop app, localhost, or an HTTPS connection.");
  }
  // Resume in the click gesture, before permissions or a first-use download.
  const context = new AudioContext();
  const resume = context.resume();
  void resume.catch(() => {});
  let stream, source, processor, mute, timer;
  const chunks = [];
  let frames = 0, closed = false;
  const close = () => {
    closed = true;
    clearTimeout(timer);
    if (processor) { processor.onaudioprocess = null; processor.disconnect(); }
    source?.disconnect(); mute?.disconnect();
    stream?.getTracks().forEach((track) => track.stop());
    if (context.state !== "closed") void context.close().catch(() => {});
  };
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true }, video: false });
    await resume;
  } catch (error) {
    close();
    if (error.name === "NotAllowedError") throw new Error("Allow microphone access in your browser or system settings to dictate.");
    throw error;
  }
  return {
    start(onLimit, onLevel) {
      if (closed) return;
      source = context.createMediaStreamSource(stream);
      processor = context.createScriptProcessor(4096, 1, 1);
      mute = context.createGain(); mute.gain.value = 0;
      processor.onaudioprocess = (event) => {
        const input = event.inputBuffer.getChannelData(0);
        const left = Math.max(0, Math.floor(context.sampleRate * 120) - frames);
        if (left) { const chunk = input.slice(0, left); chunks.push(chunk); frames += chunk.length; }
        onLevel?.(loudness(rms(input)));
      };
      source.connect(processor); processor.connect(mute); mute.connect(context.destination);
      timer = setTimeout(onLimit, 120000);
    },
    stop() { close(); return base64PCM(pcm16(chunks, context.sampleRate)); },
    cancel() { chunks.length = 0; close(); },
  };
}

export const LEVELS = 16;

// Shared by the small and large buttons. Capture the destination when recording
// starts so a late transcript cannot overwrite another conversation's draft.
// state.levels holds the latest loudness values, oldest first, for the meter.
export function createDictation({ state, request, capture, deliver, onError, onWarning }) {
  let generation = 0, recorder = null, target = null, model = "whistle";
  function cancel() {
    generation++;
    recorder?.cancel(); recorder = null;
    state.phase = "idle";
  }
  async function stop() {
    if (state.phase !== "recording") return;
    const token = generation, destination = target;
    state.phase = "transcribing";
    try {
      const mic = recorder; recorder = null;
      const pcm = await mic.stop();
      if (token !== generation) return;
      if (!pcm) throw new Error("No audio was recorded. Try speaking for a little longer.");
      const result = await request("POST", "/api/speech/transcribe", { model, pcm });
      if (token !== generation) return;
      if (!result.text?.trim()) throw new Error("No speech detected. Try again.");
      deliver(destination, result.text);
      if (result.warning) onWarning(result.warning);
    } catch (error) { if (token === generation) onError(error); }
    finally { if (token === generation) state.phase = "idle"; }
  }
  async function toggle(destination) {
    if (state.phase === "recording") return stop();
    if (state.phase !== "idle") return cancel();
    if (!destination) return;
    const token = ++generation;
    target = destination;
    state.phase = "permission";
    try {
      const mic = await capture();
      if (token !== generation) { mic.cancel(); return; }
      recorder = mic;
      const config = await request("GET", "/api/speech");
      if (token !== generation) return;
      model = config.model;
      state.phase = config.ready ? "preparing" : "downloading";
      await request("POST", "/api/speech/prepare", { model });
      if (token !== generation) return;
      state.levels = [];
      recorder.start(stop, (level) => {
        if (token === generation && state.phase === "recording") state.levels = [...state.levels.slice(1 - LEVELS), level];
      });
      state.phase = "recording";
    } catch (error) {
      if (token === generation) { cancel(); onError(error); }
    }
  }
  return { toggle, stop, cancel };
}
