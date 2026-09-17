# The tree and the changed list follow the disk

Tree and Changed used to be read when the project changed and when a turn
ended, so an editor, a `git commit` in a terminal, or a build left both panes
showing what was true minutes ago. They now follow the project folder: a file
saved outside agenttik reaches the panes in about a quarter of a second, with
no polling in between.

The server watches, the browser re-reads:

- `watchers` (`app/internal/server/watch.go`) keeps one `fsnotify` watcher per
  project, refcounted. Two windows on the same project share it; the last one
  to leave stops it.
- The SSE stream names the project the window is showing as `?watch=<id>`. That
  one parameter both subscribes to `runner.FilesTopic(id)` and holds the
  watcher open for as long as the connection lasts.
- A burst of changes becomes one `files_changed` event, and the browser answers
  it with `refreshChanged()` and `refreshTree()` — the same calls a project
  switch makes.

`git ls-files` keeps listing a tracked file that has been deleted until the
deletion is staged, so `withoutDeleted` drops those: the tree now loses a file
the moment it leaves the disk.

## Choices

**Watch on the server, not in the browser.** The File System Access API needs a
per-folder permission prompt and only reaches what the user grants. The server
already has the path.

**A topic of its own, not `ProjectTopic`.** The project topic carries the start
and end of every turn. Sending file events there would have made a window with
a task tab open receive each turn event twice — the transcript grows from
that stream.

**Announce that something moved, not what.** The panes re-read their whole
listing, which is one `git status` and one `git ls-files`. A path in the event
would only invite patching a list that git is the source of truth for.

**One event per burst.** 250 ms of quiet, and at most 2 s of waiting while
writes keep coming. A `git checkout` touching a thousand files is one refresh;
a build that writes for a minute still reports every two seconds.

**Ignored directories are never watched.** `git ls-files --others --ignored
--directory` names the subtrees to leave alone, on top of the `skipDirs` the
listing already skips. A `target/` or `dist/` can hold more files than the
project and none of them ever reach either pane, so watching one would spend
descriptors to wake the UI for nothing. Watches are capped at 4096 directories:
the allowance is shared with the user's editor. The cap bounds the walk rather
than the descriptors on macOS, where kqueue opens one per file in every watched
directory rather than one for the lot as inotify does — fsnotify must be v1.10.1
or newer, which is the first release whose `Close` hands those back.

**`.git` is watched one level deep.** Staging, committing and switching
branches move `index` or `HEAD` without touching the working tree. It is not
walked — refs and objects are noise — and `index.lock` is filtered out, because
the `index` that follows it is the event that matters.

**Attribute-only events on `.git` are dropped.** Reading `.git/index` bumps its
access time, and on macOS kqueue reports that as `NOTE_ATTRIB`, which fsnotify
delivers as `Chmod`. Refreshing on it runs `git status`, which reads the index
again: the pair never settles. Measured on a 15k-file project, it spun at about
2.5 refreshes a second with nobody touching the app, walking and serializing
the whole tree each time and holding a third of a core. `watchFilter` therefore
takes the event rather than the path and drops a `.git` entry whose op is only
`Chmod`. Staging and committing write the index, so the Changed pane still
hears what it needs, and a mode change in the working tree still counts.

**`git --no-optional-locks`.** Plain `git status` rewrites the index to cache
what it stat'd, which the watcher sees as a change. Every git call here is a
read, so the flag is set for all of them. The flag stops the write; it does not
stop the access-time bump a read leaves behind, which is why the op filter
above is needed as well. Linux needs neither half of this as visibly: inotify
does not raise `IN_ATTRIB` for a plain read, so the loop is macOS-only.
