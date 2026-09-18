# Find in the current page or text file

Ctrl+F (Cmd+F on macOS) opens a find bar above the active central view. The
shortcut can be changed in Settings → Shortcuts. Opening it again focuses and
selects the query. The query stays available when the bar is reopened.

Search is literal and case insensitive. Text files in Edit mode search their
current contents, including unsaved changes, without counting the highlighted
copy twice or including the file header. Other views search rendered text in
the central pane, excluding the sidebar, workspace, tab strip and prompt box.
Inline formatting does not interrupt a phrase. Hidden or collapsed content,
form fields, unrendered terminal scrollback and sandboxed HTML iframe contents
are not searched.

The bar shows the active match and total, or “No matches”. Enter / Shift+Enter
and the arrow buttons move forward / backward, wrapping at either end. Matches
are highlighted and the current match scrolls into view. Engines without CSS
Highlights use a DOM selection for the current match. Escape in the find bar or
the close button closes it and restores focus. Changing project, tab or file
mode closes the bar and clears its highlights.

Search builds DOM ranges without rewriting Vue's rendered content. An observer
exists only while the bar is open; content changes refresh results after a
120 ms debounce. Results reset to the first match when text or query changes.

Verified by web unit tests for literal and Unicode matching, and browser tests
for file contents, unsaved edits, wrapping, scrolling, focus, view changes and
phrases across Markdown formatting.
