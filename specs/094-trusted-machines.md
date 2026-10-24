# Trusted machines

An agenttik that signs in to another machine's exposed server with the
password (and the code, when 2FA is on) is trusted there from then on. It
signs in again by itself, with its own key, from any address: LAN, the public
tunnel, or a new IP. The server's owner can remove it in Settings → Server;
its next request then asks for the password again.

This covers remote profiles ([085](085-remote-profiles.md)) and the mobile
app ([099](099-mobile-app.md)). `agenttik --remote` and browsers have no key
and always use the password form.

## Keys

Each data directory has an Ed25519 key in `<data-dir>/machine.key` (PKCS#8
PEM, mode 0600, written by atomic rename). It is made the first time a
remote sign-in needs it. A damaged file is an error, not a reason to make a
new key silently. Private mode keeps it under the temporary root, so a
temporary instance is never trusted after it exits.

Keys travel as base64url (no padding) of the 32-byte public key. The
**fingerprint** shown in the UI is the hex of its first eight bytes.

## Becoming trusted

The password sign-in in `signInRemote` adds three form fields to the remote's
own login form: `key`, `id` (this machine ID) and `name` (this server name).
After a good password and code, the gate saves them with the client's
address. A sign-in with the same key refreshes the entry and keeps its
`added_at`. A browser sends no key and is never trusted. A malformed key is
ignored; the sign-in still succeeds.

## Signing in with the key

1. `GET /__auth/challenge` returns `{"nonce": "<64 hex>"}`. A nonce lives
   one minute and is spent by the first attempt, good or bad. At most 1024
   wait at once; more get 503.
2. `POST /__auth/key` with JSON `{key, nonce, signature, id, name}`. The
   signature is Ed25519 over
   `agenttik-key-login-v1\n<server machine ID>\n<nonce>`.
   Success is 204 with the normal session cookie; anything else is 401 JSON.

The machine ID in the message stops a signature made for one server from
being replayed against another: a remote you added cannot use your key to
sign in to a third machine. Key attempts are not counted toward the
password lockout, so a removed machine that keeps trying cannot lock its
owner out of the form. Key sign-in needs a finished lock, like the password
form ([043](043-exposed-server.md)). Both paths belong to the app when the
lock is off.

On the client, `remoteManager.request` answers a 401 by signing in with the
key and sending the request once more. Key sign-ins are serialised per
instance; a request that waited finds the new session and skips signing in.
A remote that refuses the key (a 4xx) is not asked again until a password
sign-in to it succeeds or the instance restarts. Network errors and 5xx do
not count as a refusal.

So a remote restart, an idle timeout, a changed password or seed, or a new
address costs one extra round trip, not a sign-in dialog.

## Settings → Server

With **Enable password** on, the pane lists **Trusted machines**: name,
last address, when last seen, and the key fingerprint (the full key on
hover). The trash button removes one at once; no confirmation, since the
cost is one password sign-in. Removal deletes the row and ends that
machine's sessions.

Changing the password or the seed ends every session but keeps the list:
trusted machines sign in again with their key. Remove a machine to revoke
it.

## Storage and API

`trusted_machines(public_key PK, machine_id, name, address, added_at,
last_seen_at)` in the main database, times in Unix milliseconds. A key
sign-in updates `machine_id`, `name`, `address` and `last_seen_at`.

| Method | Path | Does |
|--------|------|------|
| GET | `/api/server/trusted` | `{machines: [{key, fingerprint, machine_id, name, address, added_at, last_seen_at}]}`, most recently seen first |
| DELETE | `/api/server/trusted/:key` | Remove and end its sessions; 404 if unknown |

## Tests

`netauth` tests cover trust after a password sign-in only, browser sign-ins,
visits from a new address, replay, signatures for another server, forged
signatures, spent nonces, no lockout from key failures, half-set locks,
`Revoke` and `RevokeKey`, and key file creation. A server test runs two
instances: password and code sign-in, automatic sign-in after sessions end,
after an address change, refusal after removal, and trust again after a new
password sign-in. A browser test lists and removes a machine.
