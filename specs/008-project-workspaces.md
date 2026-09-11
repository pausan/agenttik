# Project workspaces, context tabs, and file diffs

Each project has one leading context tab in its own strip. The context tab shows either the project page or the selected task. Opening a project page or task replaces that context tab; it never adds another project or task tab. A context tab is inserted before every independent view in the project strip.

Files, commit diffs, schedules, and other independent views remain ordinary tabs. A single click still uses the temporary file tab, and a double click keeps a file tab. `Ctrl+W` still removes only the active tab, so closing a context tab does not close its independent files.

The sidebar owns active-task order. Its first nine tasks in the selected project carry numbers 1 through 9 from top to bottom. `Alt+1` through `Alt+9` select those tasks and put the cursor in their prompt. `Ctrl+PageUp` and `Ctrl+PageDown` wrap through every project and task in the sidebar, top to bottom across all of them rather than only the selected project's, so they reach tasks past nine too. `Ctrl+Tab` and `Ctrl+Shift+Tab` wrap right and left through every visible tab in the project strip.

Selecting a project still changes to that project strip. Where it lands depends on how it was reached. `Alt+A` … `Alt+H` go to the work: the task that project was last left on, which is whatever session sits in its context slot, and otherwise the first task in its sidebar list — the one `Alt+1` reaches. Only a project with no tasks at all shows its own page, which is where the first task is started from. A click on a project row shows that page instead, whether or not the project is already selected: the page is the project's overview, and once a conversation is open on top of it the row is the only way back. The tabs remain one flat array, narrowed by `S.activeProjectID` for display.

## Choices

**One context slot per project.** A project strip is the visible workspace, so one context slot per strip keeps a project page or one session at the left edge while preserving the independent tabs beside it.

**Sidebar order drives task navigation.** Task tabs no longer supply an order because only one task is visible at once. The sidebar is already the canonical project-task order and gives stable top-to-bottom numbers.

**The keyboard goes to the work, the pointer to the page.** The two ways into a project answered the same way before, and both were wrong half the time: the chord meant to drop straight into a conversation could land on a file or an overview, and the row had no way back to the page once a task was open on top of it. Splitting them gives each one job, and neither needs the other.

**No project remembers the tab it was left on.** `S.lastTab` is gone: the context slot already holds the last task opened there, and a file read in the project stays in the strip beside it, so the remembered id only ever pointed at something one of the two rules above finds on its own.

**Independent files keep their owner.** Replacing the context reassigns files from the old context to the new one. Their inspector and Tree context therefore remain useful without opening a second task tab.
