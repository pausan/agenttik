// How often an open page checks on its server, and how long one check and
// the pause before the second one take. Two failures in a row mean the server
// cannot be reached from here, not that a request was dropped on the way.
const REFRESH_MS = 5 * 60 * 1000;
const TIMEOUT_MS = 5000;
const RETRY_MS = 3000;

// The mobile app gives the server it opened a channel back to its connect
// page, so the phone can sign out and pick another server. Browsers and the
// desktop app have no such channel and get null.
export function mobileApp(scope = globalThis) {
  const channel = scope.webkit?.messageHandlers?.agenttikApp ?? scope.agenttikApp;
  if (!channel?.postMessage) return null;

  // Not through the API client: on a remote profile that would ask the other
  // machine, and the phone wants to know about the one it is connected to.
  async function ask() {
    const abort = new AbortController();
    const timer = setTimeout(() => abort.abort(), TIMEOUT_MS);
    try {
      return await scope.fetch("/api/server/addresses", { credentials: "same-origin", signal: abort.signal });
    } finally {
      clearTimeout(timer);
    }
  }

  const app = {
    // logout ends this phone's session, then hands the WebView back to the
    // connect page. The phone stays trusted, so it can sign in again by key.
    async logout(fetcher = (url, options) => scope.fetch(url, options)) {
      await fetcher("/__auth/logout", { redirect: "manual", credentials: "same-origin" }).catch(() => {});
      channel.postMessage("disconnect");
    },

    // refresh tells the app where this server can be reached now, for the
    // day this address stops working. When it already has, the app takes over
    // and tries the others: this page can only ever talk to its own origin.
    // Any answer, even a refusal, means the server is still here.
    async refresh(fetcher = ask, retry = RETRY_MS) {
      let res = await fetcher().catch(() => null);
      if (!res) {
        await new Promise((resolve) => setTimeout(resolve, retry));
        res = await fetcher().catch(() => null);
      }
      if (!res) return channel.postMessage("lost");
      const data = res.ok ? await res.json().catch(() => null) : null;
      if (Array.isArray(data?.addresses)) channel.postMessage("addresses:" + JSON.stringify(data.addresses));
    },

    // watch refreshes every REFRESH_MS while the page shows, and when the
    // phone comes back to it, which is when it may have left home.
    watch() {
      const check = () => {
        if (scope.document.visibilityState === "visible") app.refresh();
      };
      setInterval(check, REFRESH_MS);
      scope.document.addEventListener("visibilitychange", check);
    },
  };
  return app;
}
