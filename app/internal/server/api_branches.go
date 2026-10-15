package server

import (
	"regexp"
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
		Branch      string         `json:"branch"`
		Target      string         `json:"target"`
		Hash        string         `json:"hash"`
		Remote      string         `json:"remote"`
		SetUpstream bool           `json:"setUpstream"`
		Force       string         `json:"force"`
		Remotes     []remoteBranch `json:"remotes"`
	}
	if err := c.BodyParser(&body); err != nil {
		return badRequest("branch is required")
	}
	action := c.Params("action")
	if action == "merge" || action == "rebase" || action == "continue" || action == "abort" {
		return s.integrateBranches(c, root, action, body.Branch, body.Target)
	}
	if body.Branch == "" && action != "checkout" && action != "fetch" {
		return badRequest("branch is required")
	}
	if operation := gitOperation(root); operation != "" {
		return badRequest("finish or abort the existing %s first", operation)
	}
	current, err := runGit(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if action != "switch" && action != "checkout" && action != "fetch" && (err != nil || strings.TrimSpace(current) != body.Branch) {
		return badRequest("current branch changed; refresh and try again")
	}
	var args []string
	switch action {
	case "fetch":
		out, err := runGit(root, "remote")
		if err != nil {
			return badRequest("%s", err)
		}
		for _, remote := range strings.Fields(out) {
			if _, err := runGitWithTimeout(root, 2*time.Minute, "fetch", "--prune", "--no-tags", "--", remote, "+refs/heads/*:refs/remotes/"+remote+"/*"); err != nil {
				return badRequest("%s", err)
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	case "checkout":
		if !commitHash.MatchString(body.Hash) {
			return badRequest("invalid commit hash")
		}
		args = []string{"switch", "--detach", "--", body.Hash}
	case "switch":
		if _, err := runGit(root, "check-ref-format", "refs/heads/"+body.Branch); err != nil || strings.HasPrefix(body.Branch, "-") {
			return badRequest("invalid branch")
		}
		if body.Remote != "" {
			return switchRemoteBranch(c, root, body.Remote, body.Branch)
		}
		if _, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+body.Branch); err != nil {
			return badRequest("local branch does not exist")
		}
		args = []string{"switch", "--no-guess", "--", body.Branch}
	case "pull":
		args = []string{"pull", "--ff-only"}
	case "push":
		return pushBranch(c, root, body.Branch, body.SetUpstream, body.Force)
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

var objectID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// pushBranch sends HEAD to the branch's upstream, or to origin under the same
// name with setUpstream. A rejection because the remote has commits HEAD lacks,
// as after a local rebase, answers with the remote tip so the user can confirm
// a force push. force is that tip: the lease refuses to overwrite anything
// newer than what the user was shown.
func pushBranch(c *fiber.Ctx, root, branch string, setUpstream bool, force string) error {
	if force != "" && !objectID.MatchString(force) {
		return badRequest("invalid commit hash")
	}
	remote, target := "origin", "refs/heads/"+branch
	if !setUpstream {
		configured, remoteErr := runGit(root, "config", "--get", "branch."+branch+".remote")
		merge, targetErr := runGit(root, "config", "--get", "branch."+branch+".merge")
		configured, merge = strings.TrimSpace(configured), strings.TrimSpace(merge)
		missing := remoteErr != nil || configured == "" || targetErr != nil || !strings.HasPrefix(merge, "refs/heads/")
		if !missing {
			out, err := runGitWithTimeout(root, 2*time.Minute, "ls-remote", "--heads", "--", configured, merge)
			if err != nil {
				return badRequest("%s", err)
			}
			missing = strings.TrimSpace(out) == ""
		}
		if missing {
			if _, err := runGit(root, "remote", "get-url", "origin"); err != nil {
				return badRequest("current branch has no upstream branch and origin is not configured")
			}
			return c.JSON(fiber.Map{"confirmPush": true})
		}
		remote, target = configured, merge
	}
	// Explicit upstream refspec prevents push.default=matching from
	// pushing unrelated branches.
	args := []string{"push"}
	if setUpstream {
		args = append(args, "--set-upstream")
	}
	if force != "" {
		args = append(args, "--force-with-lease="+target+":"+force)
	}
	args = append(args, "--", remote, "HEAD:"+target)
	_, pushErr := runGitWithTimeout(root, 2*time.Minute, args...)
	if pushErr == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if force == "" {
		// Ancestry rather than git's message, which may be translated. A tip
		// unknown locally is not an ancestor either: the remote moved on.
		out, err := runGitWithTimeout(root, 2*time.Minute, "ls-remote", "--heads", "--", remote, target)
		tip, _, _ := strings.Cut(strings.TrimSpace(out), "\t")
		if err == nil && objectID.MatchString(tip) {
			if _, err := runGit(root, "merge-base", "--is-ancestor", tip, "HEAD"); err != nil {
				return c.JSON(fiber.Map{"confirmForce": true, "remote": remote, "tip": tip, "error": pushErr.Error()})
			}
		}
	}
	return badRequest("%s", pushErr)
}
