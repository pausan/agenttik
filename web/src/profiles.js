import { reactive } from "vue";
import { api, profileID, setPrivateMode } from "./api.js";

export const profiles = reactive({ items: [], private: false });

export async function loadProfiles() {
  const data = await api("GET", "/api/profiles");
  profiles.items = data.profiles;
  profiles.private = data.private;
  setPrivateMode(data.private);
}

export function switchProfile(id) {
  if (id === profileID) return;
  const url = new URL(window.location.href);
  if (id === "default") url.searchParams.delete("profile");
  else url.searchParams.set("profile", id);
  window.location.assign(url.href);
}

export function switchAdjacentProfile(offset) {
  const index = profiles.items.findIndex(p => p.id === profileID);
  if (index < 0 || profiles.items.length < 2) return;
  switchProfile(profiles.items[(index + offset + profiles.items.length) % profiles.items.length].id);
}
