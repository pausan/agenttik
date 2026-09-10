# Scheduled jobs

## Outcome

A prompt can be scheduled to run again and again. The Send menu holds a third
action, **Schedule**, beside Send and Enqueue. It asks for a recurrence and a
number of runs, and leaves a schedule behind in the project. When each run
comes due the schedule starts a *new* session with the same prompt, on the
provider, model, effort and permission the prompt bar was set to.

A schedule is not a session. It has no transcript, no provider session id and
no turns of its own — it is a prompt plus a clock, and what it produces are
ordinary sessions. That is why it is its own table and its own kind of tab
rather than a flag on `sessions`: nearly every list, count and view that reads
a session would otherwise need to say "unless it is a schedule".

## The recurrence

Two forms, which is what the dialog offers:

- **Every X hours Y minutes.** Defaults to 15 minutes, and whatever was
  entered last becomes the default for the next one, kept in `localStorage`
  beside the other prompt-bar choices. A run every quarter of an hour is the
  common case; typing it again each time is not.
- **Every day, week or month at hh:mm.** The time is local, because it is read
  off a clock on a wall.

Day-of-week and day-of-month are **not** asked for. They are taken from the
moment the schedule was created: a weekly schedule made on a Tuesday runs on
Tuesdays, a monthly one made on the 14th runs on the 14th. The dialog offers
exactly the three words the user gave it, and an anchor answers the rest
without another control. A monthly schedule anchored past the end of a short
month clamps to that month's last day, so the 31st still fires in February.

`next_run_at` is stored, not recomputed from the created time on every tick, so
a due schedule is one indexed comparison. Advancing it after a fire adds whole
intervals until it is in the future. That matters when the app was closed for
three hours: a 15-minute schedule catches up to the next slot instead of firing
twelve times in a row, and no skip is recorded for ticks nobody was there to
see. Skips are for work that was in the way, not for a laptop that was shut.

## The counter

Every schedule carries how many runs are left: `-1` for infinite, or a number.
It is a plain number input in the schedule's view and can be changed whenever —
from infinite to five, from five to fifty, up or down. Below `-1` clamps to
`-1`; `0` means finished, which is what the counter reaches on its own.

**The counter goes down when a run finishes, not when it starts.** A run that
ended in an error still counts: it ran, it is over, and retrying forever on a
prompt that cannot succeed is worse than stopping. A schedule with nothing left
stops scheduling and stays where it is, so its history is still readable.

A skip does not count. Nothing ran.

## Skipping

If a run comes due while this schedule's previous run is still going, the run
is **skipped** — not queued behind it. A prompt whose whole point is "do this
every 15 minutes" is not improved by running it four times back to back at
17:00 because the 16:00 one was slow. The skip is recorded with its time, so
the view says why there is a gap rather than leaving one.

The check is per schedule, not per project. Two schedules in one project are as
independent as two sessions in one project, and the project queue
([012](012-session-queue.md)) is deliberately not involved: a schedule sends
straight to its new session.

## Pause, archive and the sidebar

A schedule row in the sidebar carries **Pause** / **Resume**. Paused, nothing is
scheduled at all until it is resumed, and resuming restarts the cadence from
now rather than firing everything the pause held back.

The archive icon appears **only on a paused schedule**. Archiving something that
is still firing would hide it while it kept starting sessions, which is the one
state nothing in the UI would explain. Archived therefore implies paused, and
unarchiving leaves it paused — a schedule comes back where it was put down, not
mid-burst.

Archived or not, a schedule stays a schedule. It is a row in `schedules`, so
there is no state in which the Sessions list has to guess whether something was
a schedule or an ordinary prompt.

**The project's sidebar list holds the schedule, not its runs.** A schedule
that has fired forty times would otherwise bury the project it belongs to. A
run appears there while it is going, the way any session does, and is archived
the moment its turn ends — it leaves the project views and stays in the
Sessions list, which is what archiving already means
([007](007-session-closing.md)). The schedule's own view is where the forty are.

The Sessions tab lists schedules above sessions, so an archived one has
somewhere to be unarchived from.

## The view

A schedule tab looks like a project page ([004](004-ui.md#tabs)) and is its own
colour on the strip. It shows the prompt it will send, the recurrence in words,
when the next run is due, the runs-left input, Pause/Resume and Delete — and
under that every run it has spawned, newest first, each with its timestamp as
`YYYY-MM-DD HH:mm:ss`. A spawned run is a session, so its row opens it. A
skipped run is a row with no session and the reason it was skipped.

Those times are **local**, where the Stats panel's are UTC. The layout is the
same; the zone is not, and deliberately: a schedule is read off the clock on
the wall, so "every day at 09:00" listing its runs at 07:00 would read as
simply wrong. `isoLocal` sits beside `isoDate` in `api.js` for that.

A schedule tab has no right-hand panel. Changed and Stats describe a
conversation; a schedule has neither, and its page already carries everything
it knows.

## Data

Migration 7 adds two tables and one column:

- **schedules** — `id, project_id, title, prompt, provider, model, effort,
  permission, every, interval_minutes, at_minute, anchor_at, remaining, paused,
  next_run_at, created_at, done_at, position`. `every` is `interval`, `day`,
  `week` or `month`. `at_minute` is minutes past local midnight for the three
  calendar forms; `interval_minutes` is the whole X hours Y minutes for the
  first. `anchor_at` is the creation time, which is what fixes the weekday and
  the day of the month. `done_at` and `position` mean what they mean on a
  session: archived-at, and the order the sidebar was dragged into.
- **schedule_runs** — `id, schedule_id, session_id, status, started_at,
  ended_at`. `status` is `running`, `done`, `error`, `interrupted` or
  `skipped`. A skipped run has no `session_id`. This is the list the view
  draws, and it is why skips can be shown at all: they never become sessions.

  `interrupted` is what a restart leaves behind, and clearing those at startup
  is not optional: an open run row is what makes a schedule read as busy, so
  one left by a killed process would skip every fire after it, forever. An
  interrupted run spends nothing — it did not finish.
- **sessions.schedule_id** — 0 for an ordinary session. It is what lets a
  finishing turn find the schedule that started it without a lookup per turn.
  It is the open **run row** that decides whether a run is spent, though, not
  this column: a scheduled session the user later prompts by hand has no open
  run, so it neither spends a run nor is archived a second time.

## Firing

The runner owns a ticker, started with the server and stopped with it. Every
15 seconds it asks the store for schedules that are due — `paused = 0 AND
done_at = 0 AND remaining <> 0 AND next_run_at <= now`, one index — and for
each one either spawns, skips, or finishes. A tick that finds nothing costs one
query, which is the point of storing `next_run_at`.

Fifteen seconds is the resolution: a daily schedule set for 09:00 fires within
fifteen seconds of it. Minute-accurate to the eye, and cheap enough that the
idle cost of the feature is a query every quarter minute.

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/schedules` | `?window=&q=&project_id=&include_done=` — the sidebar list |
| POST | `/api/schedules` | `{project_id, prompt, provider, model, effort, permission, every, interval_minutes, at_minute, remaining}` |
| GET | `/api/schedules/:id` | the schedule and its runs |
| PATCH | `/api/schedules/:id` | `{title, remaining, paused, done}` — any subset |
| DELETE | `/api/schedules/:id` | drops the schedule and its run history; the sessions it spawned stay |

`GET /api/projects/:id` also carries the project's open schedules, the way it
already carries its open sessions, so the Projects sidebar needs no second
request.

## Validation

`go build ./...`, `go vet ./...`, `make ui` and the web unit tests (33) pass.
`go test ./...` fails only `TestDoneReachesProjectTopic` and
`TestReorderSessionsDrivesProjectOrder`, both of which failed before this work.

Against the running app with a fake `claude` on PATH:

- A one-minute schedule for three runs fired, its run appeared in the list as
  `Done` with its local timestamp, and the counter went 3 → 2 when the turn
  ended — not when it started.
- With a `claude` that outlasts the interval, the fire that came due behind it
  was recorded as `skipped` with no session, and the counter did not move.
- The spawned session appeared in the project's sidebar list while it ran and
  was gone from it once the turn ended, leaving the schedule as the only row.
  It is in the Sessions list, archived, where the schedule sits above it.
- Pausing showed the archive icon and hid it again on resume; the header read
  `paused` in place of the next run.
- The recurrence arithmetic, checked on a Thursday at 06:49 local: daily at
  09:00 → today 09:00, daily at 05:00 → tomorrow 05:00, weekly → the Thursday
  it was created on, monthly → the 10th, its anchor day.
- A schedule tab draws one `<aside>`, not two: it has no right-hand panel.

No console or page errors throughout.
