package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

func (s *Server) branchAction(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	if !isGitRepo(root) {
		return badRequest("not a git repository")
	}
	unlock, err := s.lockRepository(root)
	if err != nil {
		return err
	}
	defer unlock()
	var body struct {
		Branch  string         `json:"branch"`
		Target  string         `json:"target"`
		Remotes []remoteBranch `json:"remotes"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("branch is required")
	}
	action := c.Params("action")
	if action == "merge" || action == "rebase" || action == "continue" || action == "abort" {
		return s.integrateBranches(c, root, action, body.Branch, body.Target)
	}
	if body.Branch == "" {
		return badRequest("branch is required")
	}
	if operation := gitOperation(root); operation != "" {
		return badRequest("finish or abort the existing %s first", operation)
	}
	current, err := runGit(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if action != "switch" && (err != nil || strings.TrimSpace(current) != body.Branch) {
		return badRequest("current branch changed; refresh and try again")
	}
	var args []string
	switch action {
	case "switch":
		if _, err := runGit(root, "check-ref-format", "refs/heads/"+body.Branch); err != nil || strings.HasPrefix(body.Branch, "-") {
			return badRequest("invalid branch")
		}
		if _, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+body.Branch); err != nil {
			return badRequest("local branch does not exist")
		}
		args = []string{"switch", "--no-guess", "--", body.Branch}
	case "pull":
		args = []string{"pull", "--ff-only"}
	case "push":
		// Explicit upstream refspec prevents push.default=matching from
		// pushing unrelated branches.
		remote, err := runGit(root, "config", "--get", "branch."+body.Branch+".remote")
		if err != nil {
			return badRequest("current branch has no upstream remote")
		}
		target, err := runGit(root, "config", "--get", "branch."+body.Branch+".merge")
		if err != nil || !strings.HasPrefix(strings.TrimSpace(target), "refs/heads/") {
			return badRequest("current branch has no upstream branch")
		}
		args = []string{"push", "--", strings.TrimSpace(remote), "HEAD:" + strings.TrimSpace(target)}
	case "clean-remote":
		return cleanRemoteBranches(c, root, body.Remotes)
	case "check-remote":
		candidates, err := mergedRemoteBranches(root)
		if err != nil {
			return badRequest("%s", err)
		}
		return c.JSON(fiber.Map{"remotes": candidates})
	case "clean":
		deleted := []string{}
		for _, base := range []string{"main", "master"} {
			if _, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+base); err != nil {
				continue
			}
			out, err := runGit(root, "for-each-ref", "--merged=refs/heads/"+base,
				"--format=%(refname:strip=2)%09%(worktreepath)", "refs/heads/")
			if err != nil {
				return badRequest("%s", err)
			}
			for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				name, worktree, _ := strings.Cut(line, "\t")
				if name == "" || name == "main" || name == "master" || name == body.Branch || worktree != "" {
					continue
				}
				// -d checks HEAD/upstream rather than the selected merge base.
				// -D is needed here after explicitly checking main/master.
				if _, err := runGit(root, "branch", "-D", "--", name); err != nil {
					return badRequest("%s", err)
				}
				deleted = append(deleted, name)
			}
		}
		return c.JSON(fiber.Map{"deleted": deleted})
	default:
		return badRequest("unknown branch action")
	}
	if _, err := runGitWithTimeout(root, 2*time.Minute, args...); err != nil {
		return badRequest("%s", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
