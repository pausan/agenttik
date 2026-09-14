package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pausan/agenttik/app/internal/store"
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

func (s *Server) generateCommitMessage(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	var body struct {
		Provider  string `json:"provider"`
		AccountID *int64 `json:"account_id"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("invalid request")
	}
	diff, err := runGit(root, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color")
	if err != nil {
		return badRequest("%s", err.Error())
	}
	if strings.TrimSpace(diff) == "" {
		return badRequest("no staged changes")
	}
	if len(diff) > 128*1024 {
		return badRequest("staged changes are too large to generate a message; write the message manually")
	}
	if body.Provider == "" {
		for _, provider := range s.registry.All() {
			if provider.Available() == nil {
				body.Provider = provider.Name()
				break
			}
		}
	}
	accountID, err := s.chooseAccount(body.Provider, body.AccountID, store.SystemAccount, true)
	if err != nil {
		return err
	}
	message := s.runner.CommitMessage(body.Provider, accountID, diff)
	if message == "" {
		return badRequest("could not generate a commit message; try again")
	}
	return c.JSON(fiber.Map{"message": message})
}
