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
  doc.head.prepend(base);
  return (doc.doctype ? new XMLSerializer().serializeToString(doc.doctype) : "") + doc.documentElement.outerHTML;
}
