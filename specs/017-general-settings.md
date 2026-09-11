# General settings, and which key submits

Settings grew a fourth section, **General**, and it is where the dialog now
opens: the sidebar's Settings button and the launcher's Settings entry both
land there, the keyboard button still lands on Shortcuts.

It holds one question so far — does the plain Enter send the prompt or queue
it? Picking the other way round swaps the pair: Enter enqueues and `Ctrl+Enter`
sends.

The answer is not a flag kept beside the shortcuts, it *is* the shortcuts.
`setEnterDoes` writes both bindings into the same `S.keys` the handlers read,
so the General pane and the Shortcuts list are one fact shown twice and cannot
disagree — the prompt bar's hint line follows without knowing the setting
exists, because it already read the bindings. That is the same reason the
shortcut list is generated from the registry rather than written out (see 015).

The cost of storing it there is that Shortcuts can still put either chord
anywhere, which is no longer one of the two arrangements. Rather than
silently overwrite that, `enterDoes` reads the pair back as `"custom"`: neither
option is ticked and the pane names the chords actually bound, with picking
either option putting the pair back. A setting that quietly undoes what the
next pane did would be worse than one that admits the state it is in.

Both panes count their rows for the rail's filter like the other three, so
`enqueue` narrows to General 2 and Shortcuts 1.

The prompt bar's button *is* the selected default, not just its label: it reads
Enqueue and enqueues by default, and Send and sends when the pair is swapped,
each with its own icon. The caret beside it opens the Send menu (see
[012](012-task-queue.md), [028](028-scheduled-jobs.md)), which always lists
Send, Enqueue and Schedule in that order — it does not reshuffle to hide
whichever one is already on the button. It is still one field group, one pill,
one seam at the caret, because the button and its menu are one control. Only
sending is refused while a turn runs, wherever it sits, since enqueueing during
a turn is the point of it.
