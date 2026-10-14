package server

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pausan/agenttik/app/internal/store"
	"github.com/pausan/agenttik/app/internal/toolpaths"
)

// turnOutput is a result file a turn left behind: a report, a chart, a
// table. The transcript draws it as a card under the turn's reply.
type turnOutput struct {
	TurnID int64 `json:"turn_id"`
	// Path is relative to the project, or absolute outside it.
	Path string `json:"path"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
	// Modified changes when the file is rewritten, which reloads its card.
	Modified int64 `json:"modified"`
}

var outputKinds = map[string]string{
	".html": "html", ".htm": "html",
	".png": "image", ".jpg": "image", ".jpeg": "image", ".gif": "image",
	".webp": "image", ".avif": "image", ".svg": "image",
	".csv": "table", ".tsv": "table",
}

const (
	maxOutputsPerTurn  = 12
	maxOutputDirs      = 200
	maxOutputDirReads  = 5000
	outputModifiedLate = 5 * time.Second
)

type outputFile struct {
	path     string
	size     int64
	modified int64
}

// turnOutputs finds each turn's result files without storing anything: the
// folders a turn's tool calls named are listed, not walked, and a file whose
// modification time falls inside the turn belongs to it. A file rewritten
// later moves to the later turn; a deleted one leaves. Outside the project
// only files a call named itself count, since other processes write there too.
//
// Only files a reply has mentioned, in this turn or an earlier one, are
// results: "I put a note on the chart" rewrites a report presented before.
// Agents also write screenshots and scratch files to check their own work;
// those stay out, so a development session reads as it always has.
func turnOutputs(root string, turns []store.Turn, messages []store.Message) []turnOutput {
	root = filepath.Clean(root)
	named := map[int64][]string{}
	said := map[int64][]string{}
	var tracker *toolpaths.Tracker
	turn := int64(-1)
	for _, m := range messages {
		if m.Role == store.RoleAssistant && m.TurnID != 0 {
			said[m.TurnID] = append(said[m.TurnID], mentionedOutputs(root, m.Content)...)
			continue
		}
		if m.Role != store.RoleTool || m.TurnID == 0 {
			continue
		}
		if m.TurnID != turn {
			if tracker != nil {
				named[turn] = tracker.Paths()
			}
			turn, tracker = m.TurnID, toolpaths.New(root)
		}
		tracker.AddSummary(m.Content)
	}
	if tracker != nil {
		named[turn] = tracker.Paths()
	}

	listed := map[string][]outputFile{}
	stats := map[string]os.FileInfo{}
	stat := func(path string) os.FileInfo {
		info, seen := stats[path]
		if !seen {
			info, _ = os.Stat(path)
			stats[path] = info
		}
		return info
	}
	out := []turnOutput{}
	mentions := []string{}
	for _, t := range turns {
		mentions = append(mentions, said[t.ID]...)
		paths := named[t.ID]
		if len(paths) == 0 || len(mentions) == 0 {
			continue
		}
		end := t.EndedAt
		if end == 0 {
			end = time.Now().UnixMilli()
		}
		end += outputModifiedLate.Milliseconds()
		found := map[string]outputFile{}
		for _, p := range paths {
			info := stat(p)
			if info == nil {
				continue
			}
			if !inside(root, p) {
				if info.Mode().IsRegular() && outputKinds[strings.ToLower(filepath.Ext(p))] != "" {
					found[p] = outputFile{p, info.Size(), info.ModTime().UnixMilli()}
				}
				continue
			}
			dir := p
			if !info.IsDir() {
				dir = filepath.Dir(p)
			}
			if skippedDir(root, dir) {
				continue
			}
			files, seen := listed[dir]
			if !seen {
				if len(listed) >= maxOutputDirs {
					continue
				}
				files = listOutputs(dir)
				listed[dir] = files
			}
			for _, f := range files {
				found[f.path] = f
			}
		}
		files := []outputFile{}
		for _, f := range found {
			if f.modified >= t.StartedAt && f.modified <= end && mentioned(f.path, mentions) {
				files = append(files, f)
			}
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].modified != files[j].modified {
				return files[i].modified < files[j].modified
			}
			return files[i].path < files[j].path
		})
		if len(files) > maxOutputsPerTurn {
			files = files[len(files)-maxOutputsPerTurn:]
		}
		for _, f := range files {
			path := f.path
			if inside(root, path) {
				path, _ = filepath.Rel(root, path)
			}
			out = append(out, turnOutput{TurnID: t.ID, Path: filepath.ToSlash(path),
				Kind: outputKinds[strings.ToLower(filepath.Ext(path))], Size: f.size, Modified: f.modified})
		}
	}
	return out
}

// outputMention is a path with a result extension as a reply writes one: in
// a code span, a link, or prose. A trailing :line or #fragment is dropped.
var outputMention = regexp.MustCompile(`(?i)[^\s` + "`" + `'"()<>\[\]*]+\.(?:html?|png|jpe?g|gif|webp|avif|svg|csv|tsv)\b`)

// mentionedOutputs lists the result paths a reply names, relative to the
// project where they are inside it.
func mentionedOutputs(root, text string) []string {
	out := []string{}
	for _, m := range outputMention.FindAllString(text, -1) {
		m = strings.TrimPrefix(strings.TrimPrefix(m, "file://"), "./")
		if decoded, err := url.PathUnescape(m); err == nil {
			m = decoded
		}
		if filepath.IsAbs(m) && inside(root, filepath.Clean(m)) {
			m, _ = filepath.Rel(root, filepath.Clean(m))
		}
		out = append(out, filepath.ToSlash(m))
	}
	return out
}

// mentioned matches a file to a reply's paths: the same path, or one that
// ends with what the reply wrote, since replies often give only a name.
func mentioned(path string, mentions []string) bool {
	path = filepath.ToSlash(path)
	for _, m := range mentions {
		if path == m || strings.HasSuffix(path, "/"+m) {
			return true
		}
	}
	return false
}

// listOutputs reads one folder's result files, not its subfolders.
func listOutputs(dir string) []outputFile {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, _ := f.ReadDir(maxOutputDirReads)
	out := []outputFile{}
	for _, e := range entries {
		if !e.Type().IsRegular() || outputKinds[strings.ToLower(filepath.Ext(e.Name()))] == "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, outputFile{filepath.Join(dir, e.Name()), info.Size(), info.ModTime().UnixMilli()})
	}
	return out
}

func inside(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// skippedDir keeps dependency and Git folders out, as the Tree does.
func skippedDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if skipDirs[part] {
			return true
		}
	}
	return false
}
