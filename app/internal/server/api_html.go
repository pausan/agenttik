package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Keep profile and remote selection in the URL path so relative resource
// requests inherit them. Strip that prefix before the usual API routing;
// remote forwarding then sends the ordinary path without a second remote hop.
func routeHTMLScope(c *fiber.Ctx) error {
	if !strings.HasPrefix(c.Path(), "/api/html/") {
		return c.Next()
	}
	parts := strings.SplitN(strings.TrimPrefix(c.Path(), "/api/html/"), "/", 3)
	if len(parts) != 3 || !strings.HasPrefix(parts[2], "projects/") || !strings.Contains(parts[2], "/preview/") {
		return badRequest("invalid HTML preview URL")
	}
	args := c.Request().URI().QueryArgs()
	args.Set("profile", parts[1])
	args.Del("remote")
	if parts[0] != "-" {
		args.Set("remote", parts[0])
	}
	c.Path("/api/" + parts[2])
	return c.Next()
}

// previewTypes are what a page may load besides images and fonts: its
// documents, styles, scripts and the data files those scripts fetch.
var previewTypes = map[string]string{
	".css":     "text/css; charset=utf-8",
	".svg":     "image/svg+xml",
	".html":    "text/html; charset=utf-8",
	".htm":     "text/html; charset=utf-8",
	".js":      "text/javascript; charset=utf-8",
	".mjs":     "text/javascript; charset=utf-8",
	".json":    "application/json",
	".geojson": "application/geo+json",
	".csv":     "text/csv; charset=utf-8",
	".tsv":     "text/tab-separated-values; charset=utf-8",
	".txt":     "text/plain; charset=utf-8",
	".wasm":    "application/wasm",
}

// previewToken lets a page read the files under one root and nothing else.
// Its scripts run on an opaque origin, so assets answer CORS requests from
// anywhere; the token is what keeps other websites from reading them. It
// binds the project and root, so a page cannot widen its own scope.
func (s *Server) previewToken(projectID, root string) string {
	mac := hmac.New(sha256.New, s.previewKey)
	mac.Write([]byte(projectID + "\x00" + root))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

// previewRoot is the folder a preview's root segment names: the project for
// "-", or an absolute folder encoded as base64url.
func (s *Server) previewRoot(c *fiber.Ctx, scope string) (string, error) {
	if scope == "-" {
		return s.projectRoot(c)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(scope)
	if err != nil || !filepath.IsAbs(string(decoded)) {
		return "", badRequest("invalid preview root")
	}
	// Resolve like projectRoot, so resolveInRoot compares resolved paths.
	root, err := filepath.EvalSymlinks(string(decoded))
	if err != nil {
		return "", badRequest("preview folder unavailable: %v", err)
	}
	return root, nil
}

// getPreviewToken answers the app itself. It sends no CORS header, so a
// sandboxed page cannot ask for a token to another root.
func (s *Server) getPreviewToken(c *fiber.Ctx) error {
	scope := c.Query("root", "-")
	if _, err := s.previewRoot(c, scope); err != nil {
		return err
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"token": s.previewToken(c.Params("id"), scope)})
}

// projectHTMLAsset serves a previewed page and what it loads. Pages run their
// scripts in a sandbox without same-origin access, here and in the app's
// iframe, so they cannot reach the app's API or storage.
func (s *Server) projectHTMLAsset(c *fiber.Ctx) error {
	path, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return badRequest("invalid preview path")
	}
	scope := c.Params("root")
	if !hmac.Equal([]byte(c.Params("token")), []byte(s.previewToken(c.Params("id"), scope))) {
		return fiber.NewError(fiber.StatusForbidden, "invalid preview token")
	}
	root, err := s.previewRoot(c, scope)
	if err != nil {
		return err
	}
	abs, err := resolveInRoot(root, path)
	if err != nil {
		return err
	}
	kind := rawTypes[strings.ToLower(filepath.Ext(path))]
	if kind == "" {
		kind = previewTypes[strings.ToLower(filepath.Ext(path))]
	}
	if kind == "" {
		return badRequest("unsupported HTML preview asset")
	}
	data, err := rawImage(root, abs, path, "")
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, kind)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Content-Security-Policy", "sandbox allow-scripts; object-src 'none'")
	return c.Send(data)
}
