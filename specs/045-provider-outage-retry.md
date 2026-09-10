# Retrying a prompt whose provider was away

A turn that fails because the provider is unreachable no longer costs the
prompt. Three failures are read as one case — no internet under the CLI, a 5xx
or an overload behind it, a spent subscription allowance in front of it —
because all three end the same way: the same prompt, unchanged, works again
later. So the prompt goes back in the queue with a clock on it, the task shows
as **waiting**, and it starts on its own once the provider answers again.

Nothing else changes. A failure that is the request's own fault — an unknown
model, a project folder that has moved, a CLI missing from `PATH` — is still
reported against the turn exactly as before.

## What the queue holds

A held prompt is an ordinary queued prompt with three extra fields, so
everything the queue already does keeps working: it is drawn in the transcript,
counted in the sidebar's clock badge, editable, forceable and stoppable.

- `retry_at` — when to try again. 0 means the prompt is only waiting its turn.
- `retry_count` — how many attempts it has already cost, which sets the wait.
- `retry_error` — the failure that put it back, which is what the bubble says.

The wait lives on the row rather than in the process. A restart mid-outage
therefore honours the wait it was already serving instead of retrying on the
way up, and a prompt that came due while the app was down is picked up on the
first tick.

Requeueing keeps the prompt's original acceptance time, which is what returns
it to the place in the queue it left rather than behind whatever was queued
while it ran. Queue order is `created_at` for the same reason. The waiting
timer keeps counting from that moment too: the wait the user is being told
about started when they sent the prompt, not when the retry was booked.

## The wait

The request is the probe. A failed one comes back immediately and costs no
tokens, so re-running the real prompt is both cheaper and more truthful than a
separate health check that could disagree with the thing it stands for.

The first retry is a minute out, and each failure after it doubles the wait to
a fifteen-minute ceiling — a provider down for an afternoon is tried a couple
of dozen times rather than three hundred. A reset the provider names beats the
backoff: Claude Code appends the instant its allowance returns to the message
(`…usage limit reached|<unix>`), and honouring it turns a five-hour wait into
one attempt instead of three hundred against a wall.

Nothing gives up. Waiting is what was asked for, and both escapes are one
click: **Send now** on the bubble tries immediately, **Stop** drops the prompt
and returns the task to idle.

A clock ticks every fifteen seconds, asks which projects hold a prompt whose
wait is over, and hands each to the ordinary scheduler. Waiting on a provider
was never a reason to jump a queue, so a released prompt runs when its project
is free, behind nothing and ahead of nothing.

## Not starving the ready work

A session holding only held prompts has nothing to run, and the scheduler is
told the difference: it picks the session and the prompt together, and passes
over a session that is waiting out an outage. Fresh work elsewhere in the same
project runs at once rather than queueing behind a wait that may last hours.

## What the transcript keeps

An attempt that produced nothing — no prose, no tool call, no tokens — is
erased: the turn and its messages go, so an afternoon of retries leaves one
waiting bubble rather than a prompt and an error per attempt, and the turns the
task is said to have spent count only work that happened.

An attempt that did get somewhere first is kept and closed as the failure it
was. Whatever it did to the project is real, and hiding it would be a lie; the
retry then reads as the second try it is.

## Where it shows

- The status dot and the header badge say **waiting**, in amber: work accepted
  that a provider is holding back, which is neither running nor idle.
- The dashed bubble's heading names the provider it is waiting for, and a line
  under it gives the failure and a countdown to the next attempt — the whole
  answer to "is this stuck?".
- A schedule whose run hit an outage keeps its run open, so it reads as busy
  and passes over its next slot instead of piling up a run an hour for the
  length of the outage. The retry closes it. See
  [028](028-scheduled-jobs.md).
