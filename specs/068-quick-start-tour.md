# Quick Start Tour

The 12-step getting-started tour starts when the user clicks Take the Quick
Start Tour beneath “Pick a task, or a project to start one.” or opens Settings
→ Help → Start Quick Start Tour. First visits never launch it automatically.
Help participates in the settings filter.

The tour is a nonmodal bottom-right panel, with a scrollable body, Previous,
Next, Finish, minimize, and close controls. Navigation never requires completing
an action and never advances automatically. Add project and Settings remain
usable alongside the panel without trapping focus or closing when the tour is
clicked. On small screens the panel fits the viewport and minimizes automatically when
Settings or Add project opens. It can also be minimized manually to reach
controls beneath it. On desktop it sits above the prompt bar, following its
height as the prompt is resized, so Send and Enqueue remain reachable. The prompt action menu opens above the
tour.

Progress and dismissal use `agenttik.quickStart.v1` through the existing scoped
storage wrapper: per browser, instance and profile, with memory-only state in
private mode. Reload resumes an open tour. Closing or finishing keeps it closed;
both entry points restart at the beginning. No server-side onboarding record is stored.

## Walkthrough

1. Welcome and navigation.
2. First agent: Subscriptions installation/login and live connected-account
   status (provider available and account signed in); more can be added later.
3. Add project → Multiple Git Repos, base folder, URL, clone. Folder supports an
   existing clone. The public practice URL is
   `https://github.com/pausan/agenttik-helloworld.git` (kept as HTTPS); an absolute
   local repository path also
   works before publication.
4. New task and provider/model selection.
5. Enqueue instructions to create a fresh tutorial branch and `AGENTS.md`.
6. Enqueue Python calculator addition and subtraction.
7. Enqueue multiplication.
8. Enqueue division and division-by-zero handling; explain branches/worktrees.
9. Send a read-only question in a separate task while queued work continues.
10. Review commits/diffs and Changed; stage a manual edit, generate a commit
    message with the sparkle button, review, and commit.
11. Archive finished tasks and find them in the project's task search.
12. Independent project queues and a repeatable daily workflow.

All four coding prompts request a 30-second Python sleep, tests, and local
commits. The first creates a fresh tutorial branch and project instructions;
the others build on it. Nothing pushes. The question prompt requests no edits.

Copy to prompt area fills the active task's empty draft and requests focus.
The destination project is shown. Existing text, attachments and pending image
uploads prevent insertion. The tour never creates a task, clones a repository,
signs in, sends, enqueues, commits, or archives on the user's behalf.

## Practice repository and validation

The initial practice repository contains `main.py` printing Hello World,
`README.md`, and a Python `.gitignore`. It is prepared separately in the sibling folder
`../tmp-agenttik-helloworld/`, on `main`, with the public URL as its origin. It must
be uploaded before new installations can use the public clone URL. This tour
ships no calculator solution and no advanced tour.

Unit tests cover startup/dismissal storage, unavailable storage, connection
status, and draft protection. Browser tests cover manual launch, navigation,
reload, Help restart, coexistence with setup dialogs, draft insertion without
submission, local cloning and explicitly enqueuing all four prompts, and phone
sizing. The fake provider verifies that a read-only question can start in a
second task while the calculator task is running. Real provider execution
remains user-driven.
