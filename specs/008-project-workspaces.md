# Project workspaces, context tabs, and file diffs

Each project has one leading context tab in its own strip. The context tab shows the project page, the selected task or the selected scheduled job. Opening any of the three replaces that context tab; it never adds a second one. A context tab is inserted before every independent view in the project strip.

Adding the first project selects it and opens its project page, so `Ctrl+N` and `Ctrl+T` can immediately start a task. Adding another project keeps the current project selected.

Files, commit diffs, schedules, and other independent views remain ordinary tabs. A single click still uses the temporary file tab, and a double click keeps a file tab. `Ctrl+W` still removes only the active tab, so closing a context tab does not close its independent files.

The sidebar owns active-task order. Its first nine tasks in the selected project carry numbers 1 through 9 from top to bottom. `Alt+1` through `Alt+9` select those tasks and put the cursor in their prompt. `Ctrl+PageUp` and `Ctrl+PageDown` wrap through every project, scheduled job and task in the sidebar, top to bottom across all of them rather than only the selected project's, so they reach jobs and the tasks past nine too. `Ctrl+Tab` and `Ctrl+Shift+Tab` wrap right and left through every visible tab in the project strip.

Selecting a project changes to that project strip, back where it was left: whatever sits in its context slot, which is the task, scheduled job or project page last opened there. A project with no context tab yet lands on the first task in its sidebar list — the one `Alt+1` reaches — and only a project with no tasks at all shows its own page. The cursor then goes to the prompt, when what comes up has one. `Alt+A` … `Alt+H`, a click on the row of a project that is not selected, and its Command Palette entry all follow this rule, in a profile or the flat view alike ([100](100-flat-view.md)). A click on the row of the project already selected shows its page instead: once a conversation is open on top of the page, the row is the way back to it. The tabs remain one flat array, narrowed by `S.activeProjectID` for display.

Open tabs preserve their scroll positions when switching tabs or projects. File
modes keep separate positions. Closing a tab releases its scroll memory; see
[076](076-tab-scroll.md).

## Choices

**One context slot per project.** A project strip is the visible workspace, so one context slot per strip keeps a project page, one session or one scheduled job at the left edge while preserving the independent tabs beside it. A job is read the way a page is — its clock, its counter, its runs — and a strip that grew a tab per job visited would bury the work the strip is for.

**Sidebar order drives task navigation.** Task tabs no longer supply an order because only one task is visible at once. The sidebar is already the canonical project-task order and gives stable top-to-bottom numbers.

**Every way into a project goes back to where it was left.** The project page is the overview, but once work is under way it is rarely what the user came back for. The context slot already records the last thing opened in the project, page included, so returning to it needs no extra state, and the selected project's row stays the way to the page.

**No project remembers the tab it was left on.** `S.lastTab` is gone: the context slot already holds the last task, job or page opened there, and a file read in the project stays in the strip beside it, so a remembered id would only point at something the rule above finds on its own.

**Independent files keep their owner.** Replacing the context reassigns files from the old context to the new one. Their inspector and Tree context therefore remain useful without opening a second task tab.
