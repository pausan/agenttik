package server

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// maxLogCommits bounds one Logs request. The pane filters what it already has,
// so this is how far back it can look rather than a page size.
const maxLogCommits = 500

// Record and field separators for the log format. Both are control characters
// git will never emit itself, which a subject line full of punctuation cannot
// be trusted not to contain.
const (
	logRecordSep = "\x1e"
	logFieldSep  = "\x1f"
)

// commitHash guards client-supplied commit revisions passed to git. Anything but hex could be read as a flag rather than a revision.
var commitHash = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

type commitEntry struct {
	// Hash is abbreviated to the eight characters the row shows.
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	// Date is already formatted as YYYY-MM-DD HH:MM in the committer's own
	// zone: git knows the offset each commit was made in and the browser does
	// not, so formatting it here is both cheaper and more truthful.
	Date      string   `json:"date"`
	Parents   []string `json:"parents"`
	Branches  []string `json:"branches"`
	Tags      []string `json:"tags"`
	FileCount int      `json:"fileCount"`
}

type projectLog struct {
	Branches  []string `json:"branches"`
	Operation string   `json:"operation"`
	// Branch is empty on a detached HEAD, where Head is all there is to say.
	Branch  string        `json:"branch"`
	Head    string        `json:"head"`
	Commits []commitEntry `json:"commits"`
}

// projectLog is the history of the branch the project is on. A folder that is
// not a repository, and one with no commits yet, both answer with an empty log
// rather than an error: neither is a fault the pane can do anything about.
func (s *Server) projectLog(c *fiber.Ctx) error {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return err
	}
	body := projectLog{Commits: []commitEntry{}, Branches: []string{}}
	if !isGitRepo(root) {
		return c.JSON(body)
	}
	body.Operation = gitOperation(root)
	if out, err := runGit(root, "for-each-ref", "--format=%(refname:strip=2)", "refs/heads/"); err == nil {
		body.Branches = strings.Fields(out)
	}
	limit := c.QueryInt("limit", maxLogCommits)
	if limit <= 0 || limit > maxLogCommits {
		limit = maxLogCommits
	}
	if out, err := runGit(root, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		body.Branch = strings.TrimSpace(out)
	}
	if out, err := runGit(root, "rev-parse", "--short=8", "HEAD"); err == nil {
		body.Head = strings.TrimSpace(out)
	}
	args := []string{"log", "--no-color", "--topo-order", "--shortstat", "--diff-merges=first-parent", "--max-count=" + strconv.Itoa(limit),
		"--pretty=format:" + logRecordSep + "%H" + logFieldSep + "%an" + logFieldSep + "%ad" + logFieldSep + "%s" + logFieldSep + "%P",
		"--date=format:%Y-%m-%d %H:%M"}
	if c.QueryBool("graph") {
		args = append(args, "--branches", "--remotes", "--tags")
	}
	if body.Head != "" {
		args = append(args, "HEAD")
	}
	out, err := runGit(root, args...)
	if err != nil {
		return c.JSON(body)
	}
	body.Commits = parseLog(out)
	// Enumerate refs separately: ref names may contain decoration delimiters,
	// and annotated tags need their peeled commit ID.
	refs, err := runGit(root, "for-each-ref", "--format=%(objectname)%1f%(*objectname)%1f%(refname)%1f%(symref)", "refs/heads/", "refs/remotes/", "refs/tags/")
	if err == nil {
		attachLogRefs(body.Commits, refs)
	}
	return c.JSON(body)
}

// parseLog reads the record-separated log, one record per commit. The
// separator, rather than the newline between records, is what keeps a subject
// full of punctuation from being read as a record of its own.
func parseLog(out string) []commitEntry {
	commits := []commitEntry{}
	for _, record := range strings.Split(out, logRecordSep) {
		if strings.TrimSpace(record) == "" {
			continue
		}
		header, stats, _ := strings.Cut(record, "\n")
		fields := strings.Split(header, logFieldSep)
		if len(fields) < 4 {
			continue
		}
		entry := commitEntry{
			Hash:    shortHash(fields[0]),
			Author:  fields[1],
			Date:    fields[2],
			Subject: fields[3],
			Parents: []string{}, Branches: []string{}, Tags: []string{},
		}
		if len(fields) > 4 {
			for _, parent := range strings.Fields(fields[4]) {
				entry.Parents = append(entry.Parents, shortHash(parent))
			}
		}
		if match := logFileCount.FindStringSubmatch(stats); len(match) > 1 {
			entry.FileCount, _ = strconv.Atoi(match[1])
		}
		commits = append(commits, entry)
	}
	return commits
}

var logFileCount = regexp.MustCompile(`(?m)^\s*(\d+) files? changed`)

func attachLogRefs(commits []commitEntry, refs string) {
	byHash := make(map[string]*commitEntry, len(commits))
	for i := range commits {
		byHash[commits[i].Hash] = &commits[i]
	}
	for _, line := range splitLines(refs) {
		fields := strings.Split(line, logFieldSep)
		if len(fields) != 4 || fields[3] != "" {
			continue
		}
		hash := fields[0]
		if fields[1] != "" {
			hash = fields[1]
		}
		commit := byHash[shortHash(hash)]
		if commit == nil {
			continue
		}
		name := fields[2]
		if strings.HasPrefix(name, "refs/tags/") {
			commit.Tags = append(commit.Tags, strings.TrimPrefix(name, "refs/tags/"))
		} else {
			name = strings.TrimPrefix(strings.TrimPrefix(name, "refs/heads/"), "refs/remotes/")
			commit.Branches = append(commit.Branches, name)
		}
	}
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

type commitFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary"`
}

type commitDetail struct {
	Hash  string       `json:"hash"`
	Files []commitFile `json:"files"`
}

// projectCommit lists what one commit touched, which is what a log row expands
// into. Two git calls rather than one: --numstat and --name-status override
// each other, and the counts and the status letter are both worth having.
func (s *Server) projectCommit(c *fiber.Ctx) error {
	root, hash, err := s.commitTarget(c)
	if err != nil {
		return err
	}
	body := commitDetail{Hash: shortHash(hash), Files: []commitFile{}}
	out, err := runGit(root, "-c", "core.quotePath=false", "show", "--no-color",
		"--format=", "--diff-merges=first-parent", "--numstat", hash)
	if err != nil && out == "" {
		return err
	}
	byPath := map[string]int{}
	for _, line := range splitLines(out) {
		cols := strings.SplitN(line, "\t", 3)
		if len(cols) < 3 {
			continue
		}
		file := commitFile{Path: renamedTo(cols[2])}
		// A binary file is counted as "-" rather than in lines.
		file.Binary = cols[0] == "-" || cols[1] == "-"
		file.Additions, _ = strconv.Atoi(cols[0])
		file.Deletions, _ = strconv.Atoi(cols[1])
		byPath[file.Path] = len(body.Files)
		body.Files = append(body.Files, file)
	}
	status, err := runGit(root, "-c", "core.quotePath=false", "show", "--no-color",
		"--format=", "--diff-merges=first-parent", "--name-status", hash)
	if err == nil {
		for _, line := range splitLines(status) {
			cols := strings.Split(line, "\t")
			if len(cols) < 2 {
				continue
			}
			// A rename names both paths; the new one is the file that exists.
			to := cols[len(cols)-1]
			if at, ok := byPath[to]; ok {
				body.Files[at].Status = cols[0]
			}
		}
	}
	prefix := s.repositoryPrefix(c, root)
	for i := range body.Files {
		body.Files[i].Path = prefix + body.Files[i].Path
	}
	return c.JSON(body)
}

// projectCommitDiff is one file as that commit changed it. git show against a
// path works on a root commit too, where there is no parent to diff against.
func (s *Server) projectCommitDiff(c *fiber.Ctx) error {
	root, hash, err := s.commitTarget(c)
	if err != nil {
		return err
	}
	rel, err := s.repositoryFilePath(c, root)
	if err != nil {
		return err
	}
	if _, err := resolveInRoot(root, rel); err != nil {
		return err
	}
	out, _ := gitDiff(root, "-c", "core.quotePath=false", "show", "--no-color",
		"--format=", "--diff-merges=first-parent", hash, "--", rel)
	body := fileDiff{Path: rel, Partial: len(out) > maxFileBytes}
	if body.Partial {
		out = out[:maxFileBytes]
	}
	body.Diff = out
	return c.JSON(body)
}

// commitTarget resolves the project folder and checks the revision. The hash
// is the only client value handed to git as a revision, so it is matched
// against hex rather than merely escaped.
func (s *Server) commitTarget(c *fiber.Ctx) (string, string, error) {
	root, err := s.repositoryRoot(c)
	if err != nil {
		return "", "", err
	}
	if !isGitRepo(root) {
		return "", "", badRequest("%s is not a git repository", root)
	}
	hash := c.Query("hash")
	if !commitHash.MatchString(hash) {
		return "", "", badRequest("invalid commit hash")
	}
	return root, hash, nil
}

// renamedTo takes the destination of a numstat rename. git writes the pair
// either whole ("old => new") or with the common part factored out
// ("dir/{a => b}/file"), and only the destination path exists to be opened.
//
// Either side of a factored pair can be empty — "web/{ui => }/index.html" is
// a move up one level — so the result is cleaned rather than concatenated:
// without that it reads "web//index.html", which matches neither the file on
// disk nor the path --name-status reports the status letter under.
func renamedTo(raw string) string {
	i := strings.Index(raw, " => ")
	if i < 0 {
		return raw
	}
	to := raw[i+4:]
	open := strings.LastIndex(raw[:i], "{")
	if closing := strings.Index(to, "}"); open >= 0 && closing >= 0 {
		to = raw[:open] + to[:closing] + to[closing+1:]
	}
	if to = path.Clean(to); to == "." {
		return raw
	}
	return to
}
