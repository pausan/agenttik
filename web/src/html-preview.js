// A document under the project is served from the project root, so its
// links can reach siblings and parents; one outside it from its own folder,
// encoded as base64url.
export function htmlPreviewRoot(path) {
  path = path.replaceAll("\\", "/");
  if (!path.startsWith("/")) return { root: "-", path };
  const end = path.lastIndexOf("/");
  const root = btoa(String.fromCharCode(...new TextEncoder().encode(path.slice(0, end) || "/")))
    .replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
  return { root, path: path.slice(end + 1) };
}

// Routing scope lives in the path: browsers drop a base URL's query when
// resolving images, stylesheets, scripts, and nested CSS imports. The token
// comes from the server and opens this one root.
export function htmlPreviewURL(tab, { remote = "", profile = "default", token = "" } = {}) {
  const { root, path } = htmlPreviewRoot(tab.path);
  const scope = [remote || "-", profile, "projects", tab.projectID, "preview", token, root].map(encodeURIComponent).join("/");
  return `/api/html/${scope}/${path.split("/").map(encodeURIComponent).join("/")}`;
}

export function htmlPreviewDocument(text, url) {
  const doc = new DOMParser().parseFromString(text, "text/html");
  const base = doc.querySelector("base[href]") || doc.createElement("base");
  let href = url;
  try { href = new URL(base.getAttribute("href") || "", url).href; } catch { /* An incomplete edited base URL uses the file's directory. */ }
  base.setAttribute("href", href);
  const annotator = doc.createElement("script");
  annotator.textContent = ANNOTATOR;
  doc.head.prepend(base, annotator);
  return (doc.doctype ? new XMLSerializer().serializeToString(doc.doctype) : "") + doc.documentElement.outerHTML;
}

/* ANNOTATOR runs inside every previewed page and does nothing until the app
   turns it on. Then it outlines what the pointer is over, and a click
   reports that element instead of reaching the page. The page is on another
   origin, so this is the only way to learn what was clicked; what it reports
   is a description for the agent, never markup the app renders. */
const ANNOTATOR = `(() => {
  let on = false, over = null;
  const style = document.createElement("style");
  style.textContent = "[data-agenttik-over]{outline:2px solid #2a78d6!important;outline-offset:1px!important;cursor:crosshair!important}";
  const mark = (el) => { over?.removeAttribute("data-agenttik-over"); over = el; el?.setAttribute("data-agenttik-over", ""); };
  const clip = (s, n) => (s || "").replace(/\\s+/g, " ").trim().slice(0, n);
  const selector = (el) => {
    const parts = [];
    for (let n = el; n && n.nodeType === 1 && n !== document.documentElement && parts.length < 6; n = n.parentElement) {
      let part = n.tagName.toLowerCase();
      if (n.id) { parts.unshift(part + "#" + n.id); break; }
      const classes = [...n.classList].slice(0, 2);
      if (classes.length) part += "." + classes.join(".");
      const same = n.parentElement ? [...n.parentElement.children].filter((c) => c.tagName === n.tagName) : [];
      if (same.length > 1) part += ":nth-of-type(" + (same.indexOf(n) + 1) + ")";
      parts.unshift(part);
    }
    return parts.join(" > ");
  };
  const heading = (el) => {
    let found = "";
    for (const h of document.querySelectorAll("h1,h2,h3,h4,h5,h6")) {
      if (h.contains(el) || h.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING) found = h.textContent;
    }
    return clip(found, 120);
  };
  const describe = (el) => {
    const r = el.getBoundingClientRect();
    const title = [...el.children].find((c) => c.tagName.toLowerCase() === "title");
    return {
      tag: el.tagName.toLowerCase(),
      selector: selector(el),
      label: clip(el.getAttribute("aria-label") || el.getAttribute("title") || el.getAttribute("alt") || title?.textContent, 160),
      text: clip(el.textContent, 160),
      data: [...el.attributes].filter((a) => a.name.startsWith("data-") && a.name !== "data-agenttik-over")
        .slice(0, 6).map((a) => a.name + '="' + clip(a.value, 60) + '"'),
      heading: heading(el),
      rect: { x: r.left, y: r.top, width: r.width, height: r.height },
    };
  };
  const send = (message) => parent.postMessage(Object.assign({ agenttik: "annotate" }, message), "*");
  addEventListener("message", (e) => {
    if (e.source !== parent || e.data?.agenttik !== "annotate-mode") return;
    on = !!e.data.on;
    if (on) document.head.append(style); else { style.remove(); mark(null); }
  });
  addEventListener("pointerover", (e) => { if (on) mark(e.target); }, true);
  for (const type of ["pointerdown", "mousedown", "pointerup", "mouseup"]) {
    addEventListener(type, (e) => { if (on) { e.preventDefault(); e.stopPropagation(); } }, true);
  }
  addEventListener("click", (e) => {
    if (!on) return;
    e.preventDefault();
    e.stopPropagation();
    send({ element: describe(e.target) });
  }, true);
  addEventListener("keydown", (e) => { if (on && e.key === "Escape") send({ cancel: true }); }, true);
})();`;

/* annotationText is what a comment on a page adds to the prompt: where it
   was made, in words an agent can find in the file, then the comment. */
export function annotationText(path, el, comment) {
  const what = el.label || el.text;
  let where = `On \`${path}\`, at the <${el.tag}>`;
  if (what) where += ` "${what.length > 80 ? what.slice(0, 79) + "…" : what}"`;
  if (el.heading && el.heading !== what) where += ` under "${el.heading}"`;
  where += ` (\`${el.selector}\`${el.data?.length ? ", " + el.data.join(", ") : ""})`;
  return `${where}:\n${comment.trim()}`;
}
