# Working indicator

## Outcome

While the current conversation has a running turn, the transcript ends with a
clock-face progress line and elapsed seconds. It uses the server turn's start
time when available, with a local timestamp only for the short gap after send.
The line is tied to the current session and disappears as soon as its turn is
done or stopped.

## Validation

`npm run build` passed. No tests were added or run.
