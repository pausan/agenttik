# Mobile app

`mobileapp/` is a Capacitor 8 app for Android and iOS. It has one embedded
page whose only job is to reach another machine's agenttik: find or type its
address, sign in, then hand the WebView to that server's own web UI. The
phone runs no agenttik of its own.

## The page

`www/` is plain HTML, CSS and ES modules with no bundler. `lib.js` holds the
pure logic and is tested with `npm test` (Node's test runner); `app.js` holds
the views and the native calls, made through `Capacitor.nativePromise`.

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
  page in the same run does not reconnect.

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
in with the key instead. iOS has no Back; reopening the app reconnects.

## Native parts

The plugin lives in the app itself, registered by `MainActivity` on Android
and by `BridgeViewController` (`AgenttikNativePlugin.swift`) on iOS. It has
two methods: `addresses` (up, non-loopback IPv4) and `allow`.

Android allows cleartext HTTP (`usesCleartextTraffic`), since LAN servers
speak plain HTTP. iOS sets `NSAllowsArbitraryLoads` and
`NSLocalNetworkUsageDescription`.

## Builds

The `android` job in `.github/workflows/build-and-release.yml` runs the page
tests, `npx cap sync android` and `./gradlew assembleRelease` on JDK 21. It
uploads `agenttik_<tag>_android.apk` (or the short commit), which a release
attaches with the desktop binaries. `versionName` is the release version and
`versionCode` is `major·1000000 + minor·1000 + patch`. Local builds are
`0.0.0`, code 1.

The APK is signed with the keystore in the `ANDROID_KEYSTORE_BASE64`,
`ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD`
secrets. Without them it falls back to that runner's debug key, which
installs but cannot update an APK signed by another key.

iOS is not built in CI. On a Mac: `npm ci && npx cap sync ios`, then open
`ios/App/App.xcodeproj` and sign it.

Locally: `npm ci && npx cap sync android && (cd android && ./gradlew
assembleDebug)`.
