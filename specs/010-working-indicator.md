# Working indicator

While the current task has a running turn, the transcript ends with a
clock-face progress line and elapsed seconds. It uses the server turn's start
time when available, with a local timestamp only for the short gap after send.
The clock refreshes at 4 FPS, while the elapsed label remains rounded down to
whole seconds. The line is tied to the current session and disappears as soon
as its turn is done or stopped.

The provider's completion event and the persisted completion event are both
received. They are deduplicated independently, so the persisted event carrying
the final stats always clears the progress line.
