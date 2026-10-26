// The mobile app gives the server it opened a channel back to its connect
// page, so the phone can sign out and pick another server. Browsers and the
// desktop app have no such channel and get null.
export function mobileApp(scope = globalThis) {
  const channel = scope.webkit?.messageHandlers?.agenttikApp ?? scope.agenttikApp;
  if (!channel?.postMessage) return null;
  return {
    // logout ends this phone's session, then hands the WebView back to the
    // connect page. The phone stays trusted, so it can sign in again by key.
    async logout(fetcher = (url, options) => scope.fetch(url, options)) {
      await fetcher("/__auth/logout", { redirect: "manual", credentials: "same-origin" }).catch(() => {});
      channel.postMessage("disconnect");
    },
  };
}
