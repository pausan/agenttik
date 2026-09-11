/* The HTTP API and the few formatters the panels share. */

export async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  // Endpoints that only accept work answer with a bare status, whose body is
  // the reason phrase rather than JSON. Failures are always JSON.
  const isJSON = res.headers.get("content-type")?.includes("json");
  const data = text && isJSON ? JSON.parse(text) : null;
  // A 401 can only be the lock on the exposed server: agenttik's own API
  // never asks for one. The session has lapsed mid-use, so the page is
  // reloaded into the login form rather than left showing a toast on a UI
  // that can no longer load anything. See specs/043-exposed-server.md.
  if (res.status === 401) {
    window.location.reload();
    throw new Error("Signed out");
  }
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data;
}

export const nf = new Intl.NumberFormat();

export function tokens(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + "M";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return nf.format(n || 0);
}

export function duration(ms) {
  if (!ms) return "0s";
  const s = Math.round(ms / 1000);
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

export function ago(ts) {
  if (!ts) return "never";
  const s = Math.round((Date.now() - ts) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  return Math.floor(s / 86400) + "d ago";
}

// Keep the stats panel stable across browser locales. Times are UTC, matching
// the ISO date source while omitting the T, fractional seconds, and Z for a
// compact `YYYY-MM-DD HH:mm:ss` display.
export function isoDate(ts) {
  if (!ts) return "never";
  return new Date(ts).toISOString().slice(0, 19).replace("T", " ");
}

// A schedule is read off the clock on the wall — "every day at 9" means local
// 9 — so its times are local, in the same YYYY-MM-DD HH:mm:ss layout. Showing
// the next run in UTC beside a local recurrence reads as simply wrong.
export function isoLocal(ts) {
  if (!ts) return "never";
  const d = new Date(ts);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ` +
    `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

export function cost(usd) {
  return "$" + (usd || 0).toFixed(4);
}
