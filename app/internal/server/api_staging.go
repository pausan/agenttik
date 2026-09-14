package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) projectStage(c *fiber.Ctx) error   { return s.changeIndex(c, true) }
func (s *Server) projectUnstage(c *fiber.Ctx) error { return s.changeIndex(c, false) }

func (s *Server) changeIndex(c *fiber.Ctx, stage bool) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return badRequest("not a git repository")
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid request")
	}
	out, err := runGit(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return badRequest("%s", err.Error())
	}
	prefix := s.repositoryPrefix(c, root)
	paths := []string{}
	for _, f := range parseStatus(out) {
		if body.Path != "" && body.Path != prefix+f.Path {
			continue
		}
		if (stage && !f.Unstaged) || (!stage && !f.Staged) {
			continue
		}
		paths = append(paths, f.Path)
		if f.Original != "" && (!stage || !f.Staged) {
			paths = append(paths, f.Original)
		}
	}
	if len(paths) == 0 {
		return badRequest("no matching changes")
	}
	args := []string{"--literal-pathspecs", "add", "--"}
	if !stage {
		args = []string{"--literal-pathspecs", "reset", "HEAD", "--"}
		if _, err := runGit(root, "rev-parse", "--verify", "HEAD"); err != nil {
			args = []string{"--literal-pathspecs", "rm", "--cached", "-f", "--"}
		}
	}
	if _, err := runGit(root, append(args, paths...)...); err != nil {
		return badRequest("%s", err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) createCommit(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return badRequest("not a git repository")
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid request")
	}
	if strings.TrimSpace(body.Message) == "" {
		return badRequest("commit message is required")
	}
	if _, err := runGitWithTimeout(root, 2*time.Minute, "commit", "-m", body.Message); err != nil {
		return badRequest("%s", err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}
