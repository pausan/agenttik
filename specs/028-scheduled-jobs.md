# Scheduled jobs

A prompt can be scheduled to run again and again. The Send menu holds a third
action, **Schedule**, beside Send and Enqueue. It asks for a recurrence and a
number of runs, and leaves a schedule behind in the project. When each run
comes due the schedule starts a *new* task with the same prompt, on the
provider, model, effort and permission the prompt bar was set to.

A schedule is not a task. It has no transcript, no provider session id and no
turns of its own — it is a prompt plus a clock, and what it produces are
ordinary tasks. That is why it is its own table and its own kind of tab rather
than a flag on `sessions`: nearly every list, count and view that reads a
session would otherwise need to say "unless it is a schedule".

## The recurrence

Two forms, which is what the dialog offers:

- **Every X hours Y minutes.** Defaults to 15 minutes, and whatever was
  entered last becomes the default for the next one, kept in `localStorage`
  beside the other prompt-bar choices. A run every quarter of an hour is the
  common case; typing it again each time is not.
- **Every day, week or month at hh:mm.** The time is local, because it is read
  off a clock on a wall.

Day-of-week and day-of-month are **not** asked for. They are taken from the
moment the form was chosen: a weekly schedule made on a Tuesday runs on
Tuesdays, a monthly one made on the 14th runs on the 14th. The dialog offers
exactly the three words the user gave it, and an anchor answers the rest
without another control. A monthly schedule anchored past the end of a short
month clamps to that month's last day, so the 31st still fires in February.

**The recurrence is as editable as the counter.** The schedule's view offers
the same four forms the dialog does, and changing one restarts the cadence
from now — the rule a resume already follows, since both mean "from here on,
this". A prompt that turned out to be worth running hourly rather than every
quarter of an hour should not have to be made again; remaking it would leave
the runs it has already recorded behind in a schedule nobody wants.

Changing the *form* re-anchors: switched to weekly on a Thursday, it runs on
Thursdays. The anchor is when the form was chosen, and choosing it again is
choosing it again. Moving only the time leaves the anchor alone, so a weekly
schedule dragged from 09:00 to 10:00 keeps its Tuesday — otherwise the one
control the dialog deliberately does not offer would move every time the clock
beside it did.

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
independent as two tasks in one project, and the project queue
([012](012-task-queue.md)) is deliberately not involved: a schedule sends
straight to its new task.

## Pause, archive and the sidebar

A schedule row in the sidebar carries **Pause** / **Resume**. Paused, nothing is
scheduled at all until it is resumed, and resuming restarts the cadence from
now rather than firing everything the pause held back.

The archive icon appears **only on a paused schedule**. Archiving something that
is still firing would hide it while it kept starting tasks, which is the one
state nothing in the UI would explain. Archived therefore implies paused, and
unarchiving leaves it paused — a schedule comes back where it was put down, not
mid-burst.

Archived or not, a schedule stays a schedule. It is a row in `schedules`, so no
list has to guess whether something was a schedule or an ordinary prompt.

**The project's sidebar list holds the schedule, not its runs.** A schedule
that has fired forty times would otherwise bury the project it belongs to. A
run appears there while it is going, the way any task does, and is archived the
moment its turn ends — it leaves the sidebar for the grey half of the project
page's Tasks tab, which is what archiving already means
([007](007-task-closing.md)). The schedule's own view is where the forty are.

## The view

A schedule tab looks like a project page ([004](004-ui.md#tabs)) and is its own
colour on the strip. Its header carries the recurrence in words, when the next
run is due, Pause/Resume and Delete. Under it sit the prompt, the model, the
**Repeats** fields and the runs-left input; and under those every run it has
spawned, newest first, each with its timestamp as `YYYY-MM-DD HH:mm:ss`. A
spawned run is a task, so its row opens it. A skipped run is a row with no task
and the reason it was skipped.

Repeats is the dialog's own control less the number of runs: the four forms,
then hours and minutes, or a time of day. A field commits when it is left, and
the header's words and next run follow it — which is the confirmation that the
new clock took, without a Save button to press. A calendar form says beside
the time that the weekday or the day of the month is the one it was set on,
since that is the control that is not there. Both Repeats and Runs left are
only rebound to the schedule while nobody is typing in them: a fire arriving
mid-edit would otherwise take the caret with it.

A schedule stores only the half of the clock its form uses — the other is 0 —
so switching form in the view offers what the dialog would have offered, a
quarter of an hour or 09:00, rather than an interval of nothing or midnight
nobody asked for.

Those times are **local**, where the Stats panel's are UTC. The layout is the
same; the zone is not, and deliberately: a schedule is read off the clock on
the wall, so "every day at 09:00" listing its runs at 07:00 would read as
simply wrong. `isoLocal` sits beside `isoDate` in `api.js` for that.

A schedule tab has no right-hand panel. Changed and Stats describe a
task; a schedule has neither, and its page already carries everything
it knows.

## Data

Migration 7 adds two tables and one column:

- **schedules** — `id, project_id, title, prompt, provider, model, effort,
  permission, every, interval_minutes, at_minute, anchor_at, remaining, paused,
  next_run_at, created_at, done_at, position`. `every` is `interval`, `day`,
  `week` or `month`. `at_minute` is minutes past local midnight for the three
  calendar forms; `interval_minutes` is the whole X hours Y minutes for the
  first. `anchor_at` is when the form was chosen — the creation time until
  the recurrence is edited into a different form — which is what fixes the
  weekday and the day of the month. `done_at` and `position` mean what they mean on a
  session row: archived-at, and the order the sidebar was dragged into.
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
  this column: a scheduled task the user later prompts by hand has no open run,
  so it neither spends a run nor is archived a second time.

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
| PATCH | `/api/schedules/:id` | `{title, remaining, paused, done, every, interval_minutes, at_minute}` — any subset |
| DELETE | `/api/schedules/:id` | drops the schedule and its run history; the sessions it spawned stay |

The three recurrence fields move together on a PATCH, because they are one
answer: `every` names the form and the other two carry it. They are read by
the same check `POST` uses, so the two cannot drift apart, and a rejected
clock changes nothing. Both a new clock and a resume book `next_run_at` once,
after whichever came in has been applied — a request carrying both gets one
booking, off the new recurrence.

`GET /api/projects/:id` also carries the project's open schedules, the way it
already carries its open tasks, so the Projects sidebar needs no second
request.
