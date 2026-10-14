# HTML previews

HTML uses a `srcdoc` iframe sandboxed with `allow-scripts` and nothing else:
scripts run, but the page has an opaque origin, no same-origin access to the
app, no forms, popups or top-level navigation. Agents write reports whose
charts, tooltips and toggles are drawn by script, so they render as they
would in a browser. The preview reads the tab's current text, including
unsaved edits. Parsing adds a document base URL; an existing `base[href]`
resolves against that URL. An invalid base falls back to the file URL. The
original doctype is preserved.

Relative images, `srcset`, stylesheets, CSS imports, CSS `url()` resources,
fonts, classic and module scripts, and files fetched by script (JSON, CSV,
TSV, text, WASM) resolve from the HTML file's directory. Inline styles,
inline scripts, data URLs and CDN scripts still work. Linked HTML pages can
navigate inside the iframe and run their scripts too. Site-root URLs such as
`/assets/image.png` still refer to the app's server root, so built
single-page apps that use them do not load.

A clean tab on Preview follows the disk: the project's `files_changed` event
re-reads it, and a changed version redraws the page. A tab with unsaved edits
is left alone.

## Routing and tokens

`/api/html/:remote/:profile/projects/:id/preview/:token/:root/*` puts routing
scope in the path because relative requests discard query parameters.
Middleware converts that prefix to the normal project API path and routing
query before remote/profile forwarding. `-` selects the local machine and
project root. An absolute document uses its directory encoded as base64url
in `:root`. Assets cannot traverse above that root or follow symlinks outside
it.

The page's requests come from an opaque origin, so module scripts, fonts and
`fetch` need CORS, and every asset answers `Access-Control-Allow-Origin: *`.
The token is what keeps other websites from using that to read project files
from the loopback server. `GET /api/projects/:id/preview-token?root=` returns
an HMAC of the project id and root under a random per-process key; it sends
no CORS header, so a sandboxed page cannot ask for a token to another root.
An asset request with the wrong token is `403`. The server that owns the
files signs and checks the token, so remote profiles work through the usual
forwarding. Tokens die with the process; each preview asks for one when it
opens.

`GET /api/projects/:id/preview/:token/:root/*` serves working-tree images,
fonts, CSS, SVG, HTML, JavaScript, JSON, GeoJSON, CSV, TSV, text and WASM.
Other file types are refused. Each asset has the existing 16 MiB limit,
`no-store`, `nosniff`, and CSP `sandbox allow-scripts; object-src 'none'`, so
a preview URL opened directly in a browser tab also runs without the app's
origin.

## Tests

Go tests cover asset types, headers, tokens bound to their root, encoded
names, external document assets, size limits, traversal, symlink boundaries
and forwarding into a remote profile. Browser tests create their own mockup
and check actual image sizes, linked/imported CSS, background requests,
fonts, unsaved edits, inline and module scripts, a data fetch, a refused
request to the app's API, and a redraw after the file changes on disk, in
default and separate profiles. URL unit tests cover scope inheritance and
Unicode absolute paths. `make test-desktop-html` checks the same asset
requests, a module script and a data fetch in a native Wails/WebKit window
using the desktop's custom URI scheme.
