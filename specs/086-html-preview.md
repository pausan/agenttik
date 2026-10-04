# HTML previews

HTML uses a sandboxed `srcdoc` iframe with no scripts, same-origin access,
forms, popups or top-level navigation. The preview reads the tab's current
text, including unsaved edits. Parsing adds a document base URL; an existing
`base[href]` resolves against that URL. An invalid base falls back to the file
URL. The original doctype is preserved.

Relative images, `srcset`, stylesheets, CSS imports, CSS `url()` resources
and fonts resolve from the HTML file's directory. Inline styles and data
URLs still work. Linked HTML pages can navigate inside the iframe. JavaScript
applications remain static; site-root URLs such as `/assets/image.png` still
refer to the app's server root.

`/api/html/:remote/:profile/projects/:id/preview/:root/*` puts routing scope in
the path because relative requests discard query parameters. Middleware
converts that prefix to the normal project API path and routing query before
remote/profile forwarding. `-` selects the local machine and project root.
An absolute document uses its directory encoded as base64url in `:root`.
Assets cannot traverse above that root or follow symlinks outside it.

`GET /api/projects/:id/preview/:root/*` serves working-tree images, fonts, CSS,
SVG and sandboxed HTML. Scripts and other file types are refused. Each asset
has the existing 16 MiB limit, `no-store`, `nosniff`, and CSP
`sandbox; script-src 'none'; object-src 'none'`. Font loads from the iframe's
opaque origin use `Access-Control-Allow-Origin: *`. The raw image/font
endpoint keeps its existing allowlist.

Go tests cover asset types, headers, encoded names, external document assets,
size limits, traversal, symlink boundaries and forwarding into a remote
profile. Browser tests create their own mockup and check actual image sizes,
linked/imported CSS, background requests, fonts, unsaved edits and script
blocking in default and separate profiles. URL unit tests cover scope
inheritance and Unicode absolute paths.
`make test-desktop-html` checks the same asset requests and script boundary in
a native Wails/WebKit window using the desktop's custom URI scheme.
