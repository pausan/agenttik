package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type remoteBranch struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	Tip    string `json:"tip"`
}

// Remote selection makes a same-name local branch and tracks the chosen remote.
// Existing local commits are preserved: syncing only permits a fast-forward.
func switchRemoteBranch(c *fiber.Ctx, root, remote, branch string) error {
	if strings.HasPrefix(remote, "-") {
		return badRequest("invalid remote")
	}
	if _, err := runGit(root, "remote", "get-url", "--", remote); err != nil {
		return badRequest("remote does not exist")
	}
	ref := "refs/remotes/" + remote + "/" + branch
	if _, err := runGit(root, "check-ref-format", ref); err != nil {
		return badRequest("invalid remote branch")
	}
	if _, err := runGitWithTimeout(root, 2*time.Minute, "fetch", "--no-tags", "--", remote, "+refs/heads/"+branch+":"+ref); err != nil {
		return badRequest("%s", err)
	}
	// A single-branch clone needs a fetch mapping for this selected branch
	// before Git can recognize its upstream. Leave normal mappings alone.
	mappings, _ := runGit(root, "config", "--get-all", "remote."+remote+".fetch")
	mapped := false
	for _, mapping := range strings.Fields(mappings) {
		mapping = strings.TrimPrefix(mapping, "+")
		if mapping == "refs/heads/*:refs/remotes/"+remote+"/*" || mapping == "refs/heads/"+branch+":"+ref {
			mapped = true
			break
		}
	}
	if !mapped {
		if _, err := runGit(root, "config", "--add", "remote."+remote+".fetch", "+refs/heads/"+branch+":"+ref); err != nil {
			return badRequest("%s", err)
		}
	}
	if _, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		if _, err := runGit(root, "switch", "--no-guess", "--", branch); err != nil {
			return badRequest("%s", err)
		}
		if _, err := runGit(root, "merge", "--ff-only", "--", ref); err != nil {
			return badRequest("%s", err)
		}
		if _, err := runGit(root, "branch", "--set-upstream-to="+ref, "--", branch); err != nil {
			return badRequest("%s", err)
		}
	} else if _, err := runGit(root, "switch", "--track", "-c", branch, "--", ref); err != nil {
		return badRequest("%s", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// Fetch explicit heads so narrow fetch configurations cannot hide a base or
// leave stale candidates. Only compare branches within the same remote.
func mergedRemoteBranches(root string) ([]remoteBranch, error) {
	candidates := []remoteBranch{}
	out, err := runGit(root, "remote")
	if err != nil {
		return nil, err
	}
	for _, remote := range strings.Fields(out) {
		fetchURL, err := runGit(root, "remote", "get-url", remote)
		if err != nil {
			return nil, err
		}
		pushURLs, err := runGit(root, "remote", "get-url", "--push", "--all", remote)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(fetchURL) != strings.TrimSpace(pushURLs) {
			return nil, fmt.Errorf("remote %s has different fetch and push destinations; cleanup cannot safely check it", remote)
		}
		prefix := "refs/remotes/" + remote + "/"
		if _, err := runGitWithTimeout(root, 2*time.Minute, "fetch", "--prune", "--no-tags", "--", remote, "+refs/heads/*:"+prefix+"*"); err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, base := range []string{"main", "master"} {
			if _, err := runGit(root, "show-ref", "--verify", "--quiet", prefix+base); err != nil {
				continue
			}
			out, err := runGit(root, "for-each-ref", "--merged="+prefix+base, "--format=%(refname)%09%(objectname)%09%(symref)", prefix)
			if err != nil {
				return nil, err
			}
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				fields := strings.Split(line, "\t")
				if len(fields) < 2 {
					continue
				}
				name := strings.TrimPrefix(fields[0], prefix)
				if name == "main" || name == "master" || name == "HEAD" || seen[name] || (len(fields) > 2 && fields[2] != "") {
					continue
				}
				seen[name] = true
				candidates = append(candidates, remoteBranch{remote, name, fields[1]})
			}
		}
	}
	return candidates, nil
}

func cleanRemoteBranches(c *fiber.Ctx, root string, requested []remoteBranch) error {
	candidates, err := mergedRemoteBranches(root)
	if err != nil {
		return badRequest("%s", err)
	}
	// Validate the whole selection before deleting anything. The lease also
	// rejects a branch changed on the server after the final fetch.
	allowed := map[remoteBranch]bool{}
	for _, candidate := range candidates {
		allowed[candidate] = true
	}
	for _, branch := range requested {
		if !allowed[branch] {
			return badRequest("remote branch %s/%s changed or is no longer merged; run cleanup again", branch.Remote, branch.Branch)
		}
	}
	deleted := []string{}
	for _, branch := range requested {
		if !allowed[branch] {
			continue
		}
		ref := "refs/heads/" + branch.Branch
		if _, err := runGitWithTimeout(root, 2*time.Minute, "push", "--force-with-lease="+ref+":"+branch.Tip, "--", branch.Remote, ":"+ref); err != nil {
			return c.JSON(fiber.Map{"deleted": deleted, "error": err.Error()})
		}
		delete(allowed, branch)
		deleted = append(deleted, branch.Remote+"/"+branch.Branch)
	}
	return c.JSON(fiber.Map{"deleted": deleted})
}
