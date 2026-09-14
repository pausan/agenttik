package server

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Revert one status entry, including both sides of a rename.
func (s *Server) projectRevert(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil || body.Path == "" {
		return badRequest("path is required")
	}
	out, err := runGit(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return badRequest("%s", err.Error())
	}
	prefix := s.repositoryPrefix(c, root)
	for _, f := range parseStatus(out) {
		if body.Path != prefix+f.Path {
			continue
		}
		paths := []string{f.Path}
		if f.Original != "" && strings.Contains(f.Status, "R") {
			paths = append(paths, f.Original)
		}
		for _, path := range paths {
			_, abs, err := resolveEntry(root, path)
			if err != nil {
				return err
			}
			info, err := os.Lstat(abs)
			if err != nil && !os.IsNotExist(err) {
				return badRequest("%s", err.Error())
			}
			if info != nil && info.IsDir() {
				return badRequest("only files can be reverted")
			}
		}
		_, headErr := runGit(root, "rev-parse", "--verify", "HEAD")
		if f.Status == "??" || headErr != nil {
			_, abs, _ := resolveEntry(root, f.Path)
			if f.Staged {
				if _, err := runGit(root, "--literal-pathspecs", "rm", "--cached", "-f", "--", f.Path); err != nil {
					return badRequest("%s", err.Error())
				}
			}
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return badRequest("%s", err.Error())
			}
		} else {
			args := []string{"--literal-pathspecs", "restore", "--source=HEAD", "--staged", "--worktree", "--"}
			if _, err := runGit(root, append(args, paths...)...); err != nil {
				return badRequest("%s", err.Error())
			}
		}
		affected := make([]string, len(paths))
		for i, path := range paths {
			affected[i] = prefix + path
		}
		return c.JSON(fiber.Map{"paths": affected})
	}
	return badRequest("no matching changes")
}
