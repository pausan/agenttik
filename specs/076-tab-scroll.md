# Open-tab scroll positions

Every open central tab keeps its vertical and horizontal scroll position when
another tab or project is selected. Closing a tab discards its position; a
reopened tab and a fresh application window start normally. File Edit, Diff,
and Preview modes each have their own position. Positions beyond shortened
content are clamped to the available space. Explicit file-line navigation
still takes precedence.

`tab-scroll.js` stores offsets in a WeakMap keyed by the tab object. Its Vue
directive saves before a view changes or unmounts and restores after rendering.
File placeholders defer restoration until content is loaded. Project pages,
scheduled jobs, pinned prompts, files and transcripts use the same directive.

A transcript first opens at the bottom. Returning to an open transcript renders
all its rows before restoring the saved position, including zero. New output
follows the bottom only while the reader is already there.

Terminal views save the emulator's viewport line and restore it after replaying
the shell output. Terminal scrollback remains bounded by its existing buffers.
Visited HTML previews stay mounted while their tabs are open in Preview mode, preserving
scroll inside the sandboxed preview without relaxing its permissions. Other
views unmount when hidden, so ordinary tabs retain only small offset records.

`web/src/tab-scroll.test.js` covers tab and mode isolation, zero offsets,
unmounting and deferred loading. `e2e/tests/tab-scroll.spec.js` checks file
switching, modes, closing/reopening, long transcripts and sandboxed HTML;
`e2e/tests/terminals.spec.js` checks terminal scrollback restoration.
