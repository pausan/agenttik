# Speech input

The prompt bar has a microphone. An empty task also has a large, centered
microphone, including on phones. Both control one recording. Click to start,
click again to stop and transcribe the whole recording. The result is appended
to the original task's editable draft, preserving existing text and images;
it is not sent automatically. Switching tasks cannot redirect a late result.
Clicking a busy microphone cancels that UI operation and discards its result.

## Preferences and first use

General → Speech input selects Whistle (default), or multilingual Whisper
Tiny/Base/Small Q5_1 and Medium Q5_0. Changing the selection does not download.
The first microphone use downloads the selected model automatically while both
buttons show a spinner and status. Browser/system microphone permission still
applies. Cached models are shared by local profiles on the host. Preferences
are per profile and General → Reset defaults restores Whistle and no correction.

Whistle supports English, Spanish, French, Italian, German, Dutch and Polish.
Whisper supports additional languages. The microphone tooltip links the choice
to Settings → General. Microphone capture needs the desktop app, localhost, or
HTTPS when accessed remotely, except in the mobile app, which records
natively when `getUserMedia` is missing ([099](099-mobile-app.md)). Linux WebKit registers the app scheme as secure
and allows audio-only capture; macOS packaging includes the microphone usage
description. Camera capture is not enabled by the Linux microphone hook.

## Recording and execution

The browser captures mono audio through Web Audio, limits it to two minutes,
averages source samples down to 16 kHz and encodes PCM16. One JSON request sends
base64 PCM to the host, avoiding Wails/WebKit Blob upload crashes. Capture nodes,
tracks and the audio context are released on stop, cancel or error. No audio is
retained in task history. Remote clients send audio to their selected host.

While recording, both microphones show a blue voice meter: bars scaled by the
loudness of recent audio (RMS on a -55 to -10 dBFS scale), newest on the right.
The small one shows five bars beside the prompt-bar button; the large one shows
sixteen under its button and keeps that space when idle, so the button does
not move under the finger. The browser reports a level per audio buffer
(about 12 per second); the mobile app polls the native recorder.

`speech.Service` serializes downloads/transcription across local profiles.
First-use artifacts are pinned by revision, size and SHA-256; downloads use
temporary files and atomic rename. Models live in `<data-dir>/speech`.
Each transcription writes a private temporary WAV, runs a bounded native
process with no shell, then removes the WAV. No model stays resident while idle.
Shutdown cancels downloads and child processes. Cancelling the client leaves
an in-progress host download free to complete and populate the cache.

Whistle downloads its pinned Needle 3.3.3 executable for Linux x64/ARM64,
macOS ARM64 or Windows x64, plus its 16.9 MB model. Whisper weights download
on demand; `make speech` builds the embedded CPU-only `whisper.cpp` v1.9.5
helper before web/desktop packaging. It needs CMake and a C++ compiler at build
time; users need neither Python nor ffmpeg. A bare `go build` supports Whistle;
Whisper reports a missing helper when `make speech` was omitted.

## Optional correction

General offers **Correct transcription**, off by default, and a model/account
picker. It uses the `transcription` automatic action, defaulting to Luna on
Codex, then Haiku on Claude, then another available provider's small-model
policy. The choice is also shown in Models → Automatic actions.

Correction is one isolated read-only request in the temporary directory, with
no task conversation or project files. It receives only the transcript and
instructions to preserve language/meaning and return corrected text. An account
or provider failure keeps the raw transcript and reports a warning.

## API and validation

`GET/PUT /api/speech` reads/updates `{model, correct}`; GET includes the model
catalog and whether weights are cached. `POST /api/speech/prepare` takes
`{model}` and prepares first use. `POST /api/speech/transcribe` takes
`{model, pcm}` and returns `{text, warning}`. Model IDs and recording bounds
are validated server-side.

Go tests cover verified caching, invalid downloads/input, temporary-file
cleanup, serialization, shutdown, preferences/reset and isolated correction.
UI tests cover conversion, cancellation races, failures and draft ownership.
Browser tests cover the download spinner, shared controls, preferences and
the phone target. The native Linux test checks microphone API availability
without opening hardware. Tests do not run speech models or paid providers.
