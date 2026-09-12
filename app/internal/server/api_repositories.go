package server

import (
	"io/fs"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
)

// Walk once per workspace refresh, skipping dependency folders and Git internals.
// Symlink directories are not followed. Worktrees with a .git file count too.
func repositoriesIn(root string) ([]string, error) {
	repos := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && skipDirs[entry.Name()] {
			return filepath.SkipDir
		}
		if isGitRepo(path) {
			rel, _ := filepath.Rel(root, path)
			repos = append(repos, filepath.ToSlash(rel))
		}
		return nil
	})
	return repos, err
}

func (s *Server) projectRepositories(c *fiber.Ctx) error {
	root, err := s.projectRoot(c)
	if err != nil {
		return err
	}
	repos, err := repositoriesIn(root)
	if err != nil {
		return err
	}
	return c.JSON(repos)
}

// List endpoints accept a repository relative to the project. File endpoints
// locate the nearest repository from their project-relative path instead, so
// open tabs keep their repository even after the inspector selection changes.
func (s *Server) repositoryRoot(c *fiber.Ctx) (string, error) {
	root, err := s.projectRoot(c)
	if err != nil {
		return "", err
	}
	if repo := c.Query("repo"); repo != "" {
		target, err := resolveInRoot(root, repo)
		if err != nil {
			return "", err
		}
		if !isGitRepo(target) {
			return "", badRequest("not a git repository")
		}
		return target, nil
	}
	if rel := c.Query("path"); rel != "" {
		abs, err := resolveInRoot(root, rel)
		if err != nil {
			return "", err
		}
		for dir := filepath.Dir(abs); abs != root && dir != root; dir = filepath.Dir(dir) {
			if isGitRepo(dir) {
				return dir, nil
			}
		}
	}
	return root, nil
}

func (s *Server) repositoryFilePath(c *fiber.Ctx, root string) (string, error) {
	project, err := s.projectRoot(c)
	if err != nil {
		return "", err
	}
	abs, err := resolveInRoot(project, c.Query("path"))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	return filepath.ToSlash(rel), err
}

func (s *Server) repositoryPrefix(c *fiber.Ctx, root string) string {
	project, _ := s.projectRoot(c)
	rel, _ := filepath.Rel(project, root)
	if rel == "." {
		return ""
	}
	return filepath.ToSlash(rel) + "/"
}
