# Mobile app

`mobileapp/` is a Capacitor 8 app for Android and iOS. It has one embedded
page whose only job is to reach another machine's agenttik: find or type its
address, sign in, then hand the WebView to that server's own web UI. The
phone runs no agenttik of its own.

## The page

`www/` is plain HTML, CSS and ES modules with no bundler. `lib.js` holds the
pure logic; `app.js` holds the views and the native calls, made through
`Capacitor.nativePromise`. `npm test` uses Node's test runner for both the
helpers and connection flows, with a fake DOM, native bridge and timers.

- **Address.** Same rules as `--remote` ([064](064-remote-connections.md)):
  a bare `host:port` means HTTP, and paths, queries, fragments and
  credentials are refused.
- **Find agenttik.** The native plugin lists the phone's IPv4 addresses. Each
  private one's /24 is probed on port 7717 at `/api/version`, 32 at a time,
  500 ms each, with the same signature check as the desktop. The desktop
  goes on to /16 and wider; the phone stops at the /24, which takes seconds.
- **Saved.** Every server reached is saved in `localStorage`, newest first,
  with its name, machine ID and whether this phone is trusted there. A cold
  start reconnects to the newest one; Cancel stops it. Coming back to the
  page in the same run does not reconnect. Scan results that are also saved
  get an accent border and say `saved` or `remembered`.

## Signing in

All requests go through `CapacitorHttp` (native, so no CORS), with redirects
off. Both platforms put the cookies it receives into the WebView's store.

1. `GET /api/version` identifies the server.
2. `GET /` with `Accept: text/html`. The app means no lock or a live session;
   the gate's form means a lock, and its `code` field means 2FA.
3. When this phone is trusted there and the machine ID still matches, it
   signs in with its key ([094](094-trusted-machines.md)): challenge, then
   an Ed25519 signature over `agenttik-key-login-v1\n<server ID>\n<nonce>`.
4. Otherwise the page asks for the password and, with 2FA, the code, and
   posts them to `/__auth/login` with `key`, `id` and `name`, like a desktop
   remote profile. A 303 is success; anything else shows the form's reason.
   The server then trusts the phone.

The phone's identity is a random 32-hex ID in `localStorage` and the name
`agenttik on Android` or `agenttik on iOS`. Its key is a non-extractable Web
Crypto Ed25519 key pair in the page's IndexedDB. Remote pages are another
origin and cannot reach it. A WebView without Ed25519 (Android System WebView
before 137, iOS before 17) still signs in with the password; it says so and
asks again next time.

## Handing over

`AgenttikNative.allow({origin})` names the one origin that may load inside
the WebView; the page then navigates to it. The plugin's `shouldOverrideLoad`
keeps that scheme, host and port in the app; every other link takes
Capacitor's default and opens in the system browser. No `allowNavigation` is
configured, so remote pages get no Capacitor bridge.

On Android, Back walks the remote's history, then loads the connect page,
then leaves. The connect page's entry is often missing from the history
because WebView skips entries a page left without a user gesture, as the
automatic reconnect does. When a session ends while the remote is open, the
remote shows its own password form; Back and a tap on the saved server signs
in with the key instead. On the connect page Back always leaves, even with
the remote's pages behind it. iOS has no Back; reopening the app reconnects.

## Connection loss on Android

The loaded UI's event streams reconnect themselves after a network loss.
Failed images, frames and other subresources leave that UI in place. A failed
main-frame navigation or reload is different: WebView replaces the document
with its built-in error page, which cannot run agenttik's reconnect code.

The native `BridgeWebViewClient` handles network errors for the allowed
origin's main frame. It clears the allowed origin, logs the WebView error
under `Agenttik`, and loads the bundled page with
`#connection-lost=<encoded origin>`. The page removes the fragment and retries
that server after three seconds, repeating after each failed attempt. This
delay also applies when native HTTP works but WebView navigation keeps
failing. It checks discovery and the front page again, signs in by key when
needed, or asks for credentials if the session and trust are no longer valid.
Non-200 front pages are failures, not successful connections.

Cancel stops scheduled retries and prevents an in-flight attempt from
navigating later. A new attempt also invalidates the old one. Saved servers,
trust and cookies survive connection loss. Recovery does not set the
signed-out flag. Cold-start and explicit connection failures still return
to the address page without repeated retries.

`npm run test:android` exercises the actual native WebView against an isolated
HTTP fixture. Install a debug APK in a running emulator and set
`ANDROID_SERIAL=emulator-<port>`; the test refuses physical devices. It checks
plain HTTP navigation, bridge isolation, subresource failure, a failed reload
after a successful connection, automatic recovery, Cancel and logout. It uses
`adb reverse` and WebView's debug protocol, with no running agenttik service.

## Logging out

Inside the app, the remote's Settings has **Log out** in its footer. The web
UI shows it only when the `agenttikApp` channel exists, so browsers and the
desktop app never see it. It requests `/__auth/logout`, which ends the
session on a locked server, then posts `disconnect` on the channel. The
native side takes `disconnect` only from the allowed origin's main frame. It
forgets that origin and loads the connect page with `#signed-out`.

The page stores that origin as `agenttik.signedOut` and drops the hash.
While the key is set, cold starts stay on the connect page, and that saved
server says `signed out`. Opening any server clears it. The phone's key and
trust are kept, so tapping the server again signs in by key.

## Native parts

The plugin lives in the app itself, registered by `MainActivity` on Android
and by `BridgeViewController` (`AgenttikNativePlugin.swift`) on iOS. It has
two methods, `addresses` (up, non-loopback IPv4) and `allow`, the
`agenttikApp` channel above (a web message listener on Android, a script
message handler on iOS), and the microphone below.

Android allows cleartext HTTP (`usesCleartextTraffic`), since LAN servers
speak plain HTTP. iOS sets `NSAllowsArbitraryLoads` and
`NSLocalNetworkUsageDescription`.

## Microphone

A plain-HTTP page is not a secure context, so the WebView hides
`getUserMedia` and dictation ([097](097-speech-input.md)) would fail. The app
records natively instead, through a channel named `agenttikMicrophone`: on
Android a `WebViewCompat` web message listener (`Microphone.java`), on iOS a
`WKScriptMessageHandlerWithReply` in the page world. Only the allowed
origin's main frame gets answers; other origins get none (Android) or an
error (iOS).

Commands are `permission` (asks once, through the system dialog), `start`,
`level`, `stop` and `cancel`. `level` returns the RMS (0..1) of the latest
audio; the web recorder polls it every 100 ms for the voice meter. `stop` returns up to two minutes of 16 kHz mono PCM16,
base64, the same body the web recorder builds, so the server sees no
difference. Android records with `AudioRecord` (voice recognition source);
iOS taps `AVAudioEngine` and converts to 16 kHz. The web recorder uses this
only when `getUserMedia` is missing, so HTTPS servers keep the browser path.

Android declares `RECORD_AUDIO` and `MODIFY_AUDIO_SETTINGS`; the second, with
the first, lets Capacitor grant `getUserMedia` on HTTPS servers. iOS sets
`NSMicrophoneUsageDescription`.

## Icon

The app icon is the desktop one, `web/public/agenttik.svg`.
`scripts/mobile-icons.py` renders it with headless Chrome and ImageMagick
into every Android mipmap and the iOS 1024px icon; rerun it when the SVG
changes. Android gets an adaptive icon (plate as background, sparkles as
foreground, the 64-unit plate filling the 72dp visible area) plus legacy
square and round PNGs. iOS gets a full-bleed opaque square, since iOS rounds
the corners itself.

## Builds

The `android` job in `.github/workflows/build-and-release.yml` runs the page
tests, `npx cap sync android` and `./gradlew assembleRelease` on JDK 21. It
uploads `agenttik_<tag>_android.apk` (or the short commit), which a release
attaches with the desktop binaries. `versionName` is the release version and
`versionCode` is `major·1000000 + minor·1000 + patch`. Local builds are
`0.0.0`, code 1.

There is no release key yet. The release build is signed with the building
machine's Android debug key, so it is not debuggable but is meant to be
installed by hand:

```
adb install agenttik_<tag>_android.apk
```

Every CI run has a new debug key, so a newer APK will not install over an
older one (`INSTALL_FAILED_UPDATE_INCOMPATIBLE`). Run `adb uninstall
com.pausan.agenttik` first. That removes the saved servers and the phone's
key: the next sign-in asks for the password again, and the server's trusted
list keeps the old entry until it is removed in Settings → Server.

Locally, `make android` runs the page tests, syncs and builds the same
release APK with this machine's debug key, version name `$(VERSION)` and code
1, and copies it to `bin/agenttik_android.apk`.

iOS is not built in CI. On a Mac with Xcode, `make ios` runs the page tests,
syncs and builds an unsigned Release app for the iOS Simulator into
`bin/agenttik_ios-simulator.app`. A phone needs a signed build: run `npx cap
open ios` in `mobileapp/` and sign it in Xcode.
