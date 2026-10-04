// Routing scope lives in the path: browsers drop a base URL's query when
// resolving images, stylesheets, and nested CSS imports.
export function htmlPreviewURL(tab, { remote = "", profile = "default" } = {}) {
  let path = tab.path.replaceAll("\\", "/");
  let root = "-";
  if (path.startsWith("/")) {
    const end = path.lastIndexOf("/");
    root = btoa(String.fromCharCode(...new TextEncoder().encode(path.slice(0, end) || "/")))
      .replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
    path = path.slice(end + 1);
  }
  const scope = [remote || "-", profile, "projects", tab.projectID, "preview", root].map(encodeURIComponent).join("/");
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
