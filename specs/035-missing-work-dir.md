# Report a missing work directory as itself

A turn whose project folder has been renamed or deleted used to fail with
`start claude: fork/exec /home/amnio/.local/bin/claude: no such file or
directory`, naming a CLI that was installed and working. The cause is that
every provider sets the child's working directory, and the OS reports a failed
`chdir` against the binary it could not run, so `exec.Cmd.Start` returns
`ENOENT` carrying the wrong path.

`agent.TurnRequest.CheckWorkDir` now stats the directory first, and the Claude
Code, Codex and Copilot providers call it right after their `Available` check.
A stale project reaches the transcript as `work dir no longer exists: <path>`.

This also keeps 034's retry honest: a failed `chdir` satisfies
`errors.Is(err, fs.ErrNotExist)` too, so a missing project folder used to buy
a second, equally doomed `exec` attempt from a retry meant for the window when
Claude Code replaces its own executable.

An empty `WorkDir` still means "inherit the server's", and is not checked.
