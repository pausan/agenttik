# Project workspaces, context tabs, and file diffs

## Outcome

Each project has one leading context tab in its own strip. The context tab shows either the project page or the selected session. Opening a project page or session replaces that context tab; it never adds another project or session tab. A context tab is inserted before every independent view in the project strip.

Files, commit diffs, schedules, and other independent views remain ordinary tabs. A single click still uses the temporary file tab, and a double click keeps a file tab. `Ctrl+W` still removes only the active tab, so closing a context tab does not close its independent files.

The sidebar owns active-session order. Its first nine sessions in the selected project carry numbers 1 through 9 from top to bottom. `Alt+1` through `Alt+9` select those sessions and put the cursor in their prompt. `Ctrl+PageUp` and `Ctrl+PageDown` wrap through every active session in that same order, so they reach sessions past nine. `Ctrl+Tab` and `Ctrl+Shift+Tab` wrap right and left through every visible tab in the project strip.

Selecting a project still changes to that project strip. Switching back restores the last tab used there. The tabs remain one flat array, narrowed by `S.activeProjectID` for display.

## Choices

**One context slot per project.** A project strip is the visible workspace, so one context slot per strip keeps a project page or one session at the left edge while preserving the independent tabs beside it.

**Sidebar order drives session navigation.** Session tabs no longer supply an order because only one session is visible at once. The sidebar is already the canonical project-session order and gives stable top-to-bottom numbers.

**Independent files keep their owner.** Replacing the context reassigns files from the old context to the new one. Their inspector and Tree context therefore remain useful without opening a second session tab.

## Validation

No tests were run, per the prototype policy.
