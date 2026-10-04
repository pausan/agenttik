package server

import (
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

// Only passive assets and sandboxed documents are served on the app origin.
// The existing raw endpoint keeps its narrower image/font allowlist.
func (s *Server) projectHTMLAsset(c *fiber.Ctx) error {
	path, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return badRequest("invalid preview path")
	}
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	if scope := c.Params("root"); scope != "-" {
		decoded, err := base64.RawURLEncoding.DecodeString(scope)
		if err != nil || !filepath.IsAbs(string(decoded)) {
			return badRequest("invalid preview root")
		}
		root = filepath.Clean(string(decoded))
	}
	abs, err := resolveInRoot(root, path)
	if err != nil {
		return err
	}
	kind := rawTypes[strings.ToLower(filepath.Ext(path))]
	if kind == "" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".css":
			kind = "text/css; charset=utf-8"
		case ".svg":
			kind = "image/svg+xml"
		case ".html", ".htm":
			kind = "text/html; charset=utf-8"
		default:
			return badRequest("unsupported HTML preview asset")
		}
	}
	data, err := rawImage(root, abs, path, "")
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, kind)
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(kind, "font/") {
		c.Set("Access-Control-Allow-Origin", "*") // Fonts load from an opaque sandbox origin.
	}
	c.Set("Content-Security-Policy", "sandbox; script-src 'none'; object-src 'none'")
	return c.Send(data)
}
