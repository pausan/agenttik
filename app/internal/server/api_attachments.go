package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/attachments"
)

// The body is the image itself, avoiding base64 copies and multipart files.
func (s *Server) uploadAttachment(c *fiber.Ctx) error {
	data := c.Body()
	if len(data) == 0 || len(data) > 4*1024*1024 {
		return badRequest("images must be between 1 byte and 4 MiB")
	}
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}[http.DetectContentType(data)]
	if ext == "" {
		return badRequest("paste a PNG, JPEG, GIF or WebP image")
	}
	name := fmt.Sprintf("%x.%s", sha256.Sum256(data), ext)
	dir := filepath.Join(s.store.Dir(), "attachments")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"url": "/api/attachments/" + name})
}

func (s *Server) getAttachment(c *fiber.Ctx) error {
	name := c.Params("name")
	if !attachments.Name.MatchString(name) {
		return fiber.ErrNotFound
	}
	c.Set("X-Content-Type-Options", "nosniff")
	return c.SendFile(filepath.Join(s.store.Dir(), "attachments", name))
}
