import { reactive } from "vue";
import { api, profileID, remoteID, setPrivateMode } from "./api.js";

// remotes are the saved remote machines, each with the profiles it listed
// when last reached. connecting drives the remote dialog: the machine and
// profile being opened (null only checks), the last connection state, and
// whether this window already shows that remote.
export const profiles = reactive({ items: [], private: false, name: "", remotes: [], connecting: null });

export async function loadProfiles() {
  const [data, saved] = await Promise.all([api("GET", "/api/profiles"), api("GET", "/api/remotes")]);
  profiles.items = data.profiles;
  profiles.private = data.private;
  profiles.name = data.name || "";
  profiles.remotes = saved.remotes;
  setPrivateMode(data.private);
}

// Every profile in picker order: this machine's, then each remote's.
export function allProfiles() {
  return [
    ...profiles.items.map(p => ({ ...p, remote: "" })),
    ...profiles.remotes.flatMap(r => r.profiles.map(p => ({ ...p, remote: r.id, machine: r.name }))),
  ];
}

export function isCurrentProfile(id, remote = "") {
  return id === profileID && remote === remoteID;
}

export function profileURL(id, remote = "") {
  const url = new URL(window.location.href);
  if (remote) url.searchParams.set("remote", remote);
  else url.searchParams.delete("remote");
  if (id === "default") url.searchParams.delete("profile");
  else url.searchParams.set("profile", id);
  return url.href;
}

// A remote profile opens only after its machine answers, so a missing
// machine shows the dialog instead of a window full of failed requests.
export function switchProfile(id, remote = "") {
  if (isCurrentProfile(id, remote)) return;
  if (!remote) {
    window.location.assign(profileURL(id));
    return;
  }
  profiles.connecting = { remote, profile: id, state: null, current: false };
  return connectRemote();
}

export function switchAdjacentProfile(offset) {
  const list = allProfiles();
  const index = list.findIndex(p => isCurrentProfile(p.id, p.remote));
  if (index < 0 || list.length < 2) return;
  const next = list[(index + offset + list.length) % list.length];
  switchProfile(next.id, next.remote);
}

// checkRemote refreshes a machine's profiles without opening one. With
// current, it checks the machine this window shows, at startup or when the
// remote asks for sign-in mid-use.
export function checkRemote(remote, current = false) {
  if (profiles.connecting) return;
  profiles.connecting = { remote, profile: current ? profileID : null, state: null, current };
  return connectRemote();
}

// connectRemote asks the saved machine for its profiles. A good answer opens
// the requested profile, or the remote's first if that one is gone.
export async function connectRemote() {
  const c = profiles.connecting;
  if (!c) return;
  c.state = { status: "checking" };
  let state;
  try {
    state = await api("POST", `/api/remotes/${encodeURIComponent(c.remote)}/connect`);
  } catch (e) {
    state = { status: "unreachable", error: e.message };
  }
  connected(c, state);
}

export function connected(c, state) {
  if (profiles.connecting !== c) return;
  if (state.remote) {
    const index = profiles.remotes.findIndex(r => r.id === state.remote.id);
    if (index < 0) profiles.remotes.push(state.remote);
    else profiles.remotes[index] = state.remote;
  }
  c.state = state;
  if (state.status !== "ok") {
    c.shown = true;
    return;
  }
  // A window on this remote reloads only if it was held up by the dialog.
  if (c.current) {
    if (c.shown) window.location.reload();
    else profiles.connecting = null;
    return;
  }
  if (c.profile === null) {
    profiles.connecting = null;
    return;
  }
  const listed = state.remote.profiles;
  const id = listed.some(p => p.id === c.profile) ? c.profile : listed[0]?.id || "default";
  window.location.assign(profileURL(id, c.remote));
}
