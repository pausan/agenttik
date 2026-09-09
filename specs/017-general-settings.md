# General settings, and which key submits

## Outcome

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

The prompt bar's primary button label follows the selected default: it reads Send
normally and Enqueue when plain Enter queues. Its Enqueue caret is left alone.

## Validation

`make ui`, `go build ./...`, `go vet ./...` and the web unit tests (32) passed.

Browser-checked headlessly against a fake `claude` on PATH that sleeps, so the
first turn stays running and the scheduler leaves the queue alone: the Settings
button opens on General and the keyboard button on Shortcuts; the rail reads
General | Appearance | Models | Shortcuts; the hint under the prompt box reads
`Enter` to send before the swap and `Ctrl+Enter` to send after it; after the
swap the Shortcuts rows read Send `Ctrl+Enter` and Enqueue `Enter`; in the box,
`Ctrl+Enter` sends and plain Enter queues the next prompt, which the transcript
draws as waiting; the choice survives a reload; filtering `enqueue` leaves
General 2 and Shortcuts 1 and `zzzz` empties the rail; rebinding Send to
`Ctrl+Shift+E` in Shortcuts leaves neither option ticked and the pane says
which chords are in use; picking one restores `Enter` and `Ctrl+Enter`. No
console or page errors.
