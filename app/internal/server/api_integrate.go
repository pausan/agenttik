package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Shared by Git mutations, including requests from other windows and projects
// pointing at the same checkout. TryLock keeps a second click from queuing work.
func (s *Server) lockRepository(root string) (func(), error) {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	entry, _ := s.gitLocks.LoadOrStore(root, &sync.Mutex{})
	lock := entry.(*sync.Mutex)
	if !lock.TryLock() {
		return nil, badRequest("a Git operation is already running in this repository")
	}
	return lock.Unlock, nil
}

func gitOperation(root string) string {
	dir, err := runGit(root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	for _, entry := range []struct{ path, action string }{{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"MERGE_HEAD", "merge"}} {
		if _, err := os.Stat(filepath.Join(strings.TrimSpace(dir), entry.path)); err == nil {
			return entry.action
		}
	}
	return ""
}

func validLocalBranch(root, branch string) bool {
	if branch == "" || strings.HasPrefix(branch, "-") {
		return false
	}
	if _, err := runGit(root, "check-ref-format", "refs/heads/"+branch); err != nil {
		return false
	}
	_, err := runGit(root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func (s *Server) integrateBranches(c *fiber.Ctx, root, action, branch, target string) error {
	operation := gitOperation(root)
	if action == "abort" {
		if operation == "" {
			return badRequest("no merge or rebase to abort")
		}
		if _, err := runGit(root, operation, "--abort"); err != nil {
			return badRequest("%s", err)
		}
		return c.JSON(fiber.Map{"message": "Operation aborted."})
	}
	if action == "continue" {
		if operation == "" {
			return badRequest("no merge or rebase to continue")
		}
		action = operation
	} else {
		if operation != "" {
			return badRequest("finish or abort the existing %s first", operation)
		}
		if branch == target || !validLocalBranch(root, branch) || !validLocalBranch(root, target) {
			return badRequest("select two different existing local branches")
		}
		current, err := runGit(root, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil || strings.TrimSpace(current) != branch {
			return badRequest("current branch changed; refresh and try again")
		}
		status, err := runGit(root, "status", "--porcelain=v1", "--untracked-files=all")
		if err != nil {
			return badRequest("%s", err)
		}
		if status != "" {
			return badRequest("commit or stash changes before merging or rebasing")
		}
		var args []string
		if action == "merge" {
			if _, err := runGit(root, "switch", "--no-guess", "--", target); err != nil {
				return badRequest("%s", err)
			}
			args = []string{"merge", "--no-edit", "--", "refs/heads/" + branch}
		} else {
			args = []string{"rebase", "--", "refs/heads/" + target}
		}
		if _, err := runGitWithTimeout(root, 2*time.Minute, args...); err != nil && gitOperation(root) != action {
			return badRequest("%s", err)
		}
	}
	if gitOperation(root) == action {
		if err := s.finishIntegration(root, action); err != nil {
			return badRequest("%s paused: %s. Retry solving conflicts or abort below.", action, err)
		}
	}
	return c.JSON(fiber.Map{"message": strings.ToUpper(action[:1]) + action[1:] + " completed."})
}

func (s *Server) finishIntegration(root, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	// Rebase can stop at a different conflict on each replayed commit.
	for step := 0; step < 100; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if gitOperation(root) != action {
			return fmt.Errorf("Git operation changed unexpectedly")
		}
		conflicts, err := runGit(root, "diff", "--name-only", "--diff-filter=U", "-z")
		if err != nil {
			return err
		}
		if conflicts != "" {
			choice, err := s.actionModel("merge")
			if err != nil {
				return err
			}
			head, err := runGit(root, "rev-parse", "HEAD")
			if err != nil {
				return err
			}
			paths := strings.Split(strings.TrimSuffix(conflicts, "\x00"), "\x00")
			// A model must not sneak unrelated edits into the merge commit.
			outside, err := integrationOutsideChanges(root, paths)
			if err != nil {
				return err
			}
			if err := s.runner.ResolveConflicts(ctx, choice, root, paths); err != nil {
				return err
			}

			after, err := runGit(root, "rev-parse", "HEAD")
			if err != nil {
				return err
			}
			if head != after || gitOperation(root) != action {
				return fmt.Errorf("HEAD or Git operation changed during conflict resolution")
			}
			outsideAfter, err := integrationOutsideChanges(root, paths)
			if err != nil {
				return err
			}
			if outside != outsideAfter {
				return fmt.Errorf("files outside the conflicts changed; review them before continuing")
			}
			// Check before staging so a failed resolution keeps the unmerged
			// index and Retry can ask the model about the same conflict again.
			if out, err := runGit(root, "diff", "--check"); err != nil {
				return fmt.Errorf("check conflict resolution: %s %w", out, err)
			}
			args := append([]string{"--literal-pathspecs", "add", "--"}, paths...)
			if _, err := runGit(root, args...); err != nil {
				return err
			}
			unresolved, err := runGit(root, "diff", "--name-only", "--diff-filter=U", "-z")
			if err != nil {
				return err
			}
			if unresolved != "" {
				return fmt.Errorf("the model left unresolved conflicts")
			}
		}
		if out, err := runGit(root, "diff", "--cached", "--check"); err != nil {
			return fmt.Errorf("check resolved changes: %s %w", out, err)
		}
		// Do not commit unstaged model edits or silently lose them on rebase.
		if _, err := runGit(root, "diff", "--quiet"); err != nil {
			return fmt.Errorf("unstaged edits remain; stage the intended resolution before retrying")
		}
		_, err = runGitWithTimeout(root, 2*time.Minute, "-c", "core.editor=true", action, "--continue")
		if gitOperation(root) == "" {
			if err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			conflicts, checkErr := runGit(root, "diff", "--name-only", "--diff-filter=U", "-z")
			if checkErr != nil || conflicts == "" {
				return err
			}
		}
	}
	return fmt.Errorf("too many conflict rounds; retry to continue")
}

// HEAD includes both staged and unstaged content; exclusions are literal even
// for filenames containing glob characters. Include new files in the check.
func integrationOutsideChanges(root string, paths []string) (string, error) {
	args := []string{"diff", "--binary", "--no-ext-diff", "--no-textconv", "HEAD", "--", "."}
	for _, path := range paths {
		args = append(args, ":(exclude,literal)"+path)
	}
	diff, err := runGit(root, args...)
	if err != nil {
		return "", err
	}
	untracked, err := runGit(root, "ls-files", "--others", "--exclude-standard", "-z")
	return diff + "\x00" + untracked, err
}
