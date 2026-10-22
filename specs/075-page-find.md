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
The bar paints and takes focus before anything else runs. Only then does a long
transcript mount its full history, which it keeps until the bar closes, so
matches include earlier messages. Collapsed tool calls and closed threads
remain excluded. Closing find releases message bodies outside the viewport.

The bar shows the active match and total, or “No matches”. In the search field,
Down / Enter moves to the next match and Up / Shift+Enter moves to the previous
match. The arrow buttons do the same. Navigation wraps at either end and keeps
the query unchanged; with no matches it does nothing. Matches
are highlighted and the current match scrolls into view. Engines without CSS
Highlights use a DOM selection for the current match. Escape in the find bar or
the close button closes it and restores focus. Changing project, tab or file
mode closes the bar and clears its highlights.

Search builds DOM ranges without rewriting Vue's rendered content. An observer
exists only while the bar is open; content changes refresh results after a
120 ms debounce. A new query resets to the first match and scrolls to it. A
content refresh keeps the current match number and does not scroll, so reading
or stepping through matches is never pulled back. In a large text file it also
leaves the caret alone. Ticking clocks (the working timer, queued-prompt waits,
approval countdowns) carry `data-find-ignore`: they are not searched, and their
updates trigger no refresh.

Verified by web unit tests for literal and Unicode matching, and browser tests
for file contents, unsaved edits, wrapping, scrolling, focus, view changes and
phrases across Markdown formatting, plus a 2,000-message transcript that checks
the bar appears within 250 ms and that content changes keep the current match
and scroll position.
