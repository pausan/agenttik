# Scheduled jobs

A prompt can be scheduled to run again and again. Manual-only saved prompts
use the same storage with `every = pinned`; see [062](062-pinned-prompts.md). The Send menu holds a third
action, **Schedule**, beside Send and Enqueue. It asks for a recurrence and a
number of runs, and leaves a schedule behind in the project. When each run
comes due the schedule creates a *new* task and queues its prompt, on the
provider, model, effort and permission the prompt bar was set to. It starts
when the project's active turns finish, following the ordinary task queue.

A schedule is not a task. It has no transcript, no provider session id and no
turns of its own — it is a prompt plus a clock, and what it produces are
ordinary tasks. That is why it is its own table and its own kind of tab rather
than a flag on `sessions`: nearly every list, count and view that reads a
session would otherwise need to say "unless it is a schedule".

## The name

A job names itself the way a task does ([020](020-task-titles.md)). The
prompt's first line, trimmed to 80 characters, is the title the moment the
dialog is submitted, so nothing is ever drawn `Untitled schedule`, and a
better name is fetched behind it from the provider's lightest model and put in
its place a few seconds later. The same isolated, read-only request a task
uses — no project files, no session, no transcript — so the two cannot drift
apart; `runner.askTitle` is the one that makes it.

**Only the placeholder is replaced.** Renaming the job by hand while the
request is in flight keeps the name that was chosen, and a job created with a
title of its own is never guessed at.

The dialog does not ask for a name. What a job is called is worth changing and
not worth typing before it exists: the common case is a prompt that is already
its own description, and the rare one is a rename. **F2** is that rename, the
same chord and the same in-place field a task's title uses
([004](004-ui.md#tabs)) — it edits the job's sidebar row whenever the job's
page is the tab in front, so the row's pencil and the keyboard reach one
control rather than two.

The title does not follow the prompt afterwards. It is named once, from the
first prompt, and renamed by hand after that, so a job someone has already
named does not lose that name to an edit.

## The number

A job is `schedules.id`, and that number is on screen: `#7` beside its name on
its own page and in the sidebar, and `#7` again on every task it has spawned.
`sessions.schedule_id` already recorded which job a run came from
([Data](#data)); drawing it is what makes the relation readable, since forty
runs of one job otherwise share a name and nothing else. The badge on a run
opens the job it belongs to, where its run history lives. Finished runs stay
open until the user archives them.

It is the row id rather than a code of its own: it is already unique, already
stored on both sides, and a second identifier would only be a thing to keep in
step with the first.

## The recurrence

The create dialog and the schedule view offer the same recurrence forms:

- **Every X hours Y minutes.** Defaults to 15 minutes. The last choices are
  saved in `localStorage` beside the other prompt-bar choices.
- **Every day at hh:mm.** Runs once each calendar day.
- **Every N days at hh:mm.** N is a whole number from 1 to 36500. The date the
  form was chosen is the first eligible date, followed by every Nth calendar
  date. If today's time has passed, the first run is N days later.
- **Selected weekdays at hh:mm.** Choose any nonempty set of weekdays, such
  as Monday, Wednesday and Friday. Each chosen day runs once a week.
- **Every week or month at hh:mm.** Uses the weekday or day of the month the
  form was chosen. A monthly anchor past the end of a short month clamps to
  that month's last day, so the 31st still fires in February.

Calendar times use the **server's local timezone**, including when the client
is remote. The forms say this next to the time field. N-day spacing counts
calendar dates rather than 24-hour durations, so daylight-saving changes keep
an ordinary wall-clock time such as 09:00. Times inside a daylight-saving gap
or repeated hour follow Go's local `time.Date` normalization.

Editing a recurrence rebooks its next future slot. Changing the form or N-day
spacing anchors it on today. Changing only its time keeps its anchor and
selected weekdays. Resuming also books the next future slot and keeps the
anchor. Runs already created and the run history stay with the schedule.

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

Neither does a run forced from the schedule's own view
([below](#running-one-now)): asking for it by hand is not the recurrence
asking for it.

## The finish condition

A job can carry a **finish condition** besides its counter: a sentence such
as *Stop when todo.txt does not exist or is empty*. It suits a job that eats a
list one item per run, where nobody knows the count in advance.

When set, the condition is the very first thing in each run's prompt. The
agent is told to check it before anything else and, if it is true, to do
nothing more and reply with only `AGENTTIK_JOB_FINISHED`. Otherwise it carries
on with the prompt below. `runner.schedulePrompt` builds the text, so a timed
run and a forced run send the same prompt. The run's task shows the prompt as
sent, condition included.

When the run's turn ends, its last reply is read. If a line of it is the
marker alone (Markdown backticks or bold around it are allowed), the run
closes as `finished` and the job **pauses itself**. That holds for forced runs
too. A reply that only quotes the instruction does not count. The run still
spends the counter the way a `done` one does. Resuming the job by hand starts
it again, and the next run checks the condition again.

The dialog has a **Stop when a condition is met** switch that reveals the
field. It cannot submit an empty condition. The condition is not remembered
for the next dialog. The job view shows it as **Finish when**. The field
commits when it is left, and an empty field turns the check off. Pinned
prompts have no condition.

## Skipping

If a run comes due while this schedule's previous prompt is queued, running,
or waiting for a provider retry, the new run is **skipped**. A prompt whose
whole point is "do this every 15 minutes" is not improved by running it four times back to back at
17:00 because the 16:00 one was slow. The skip is recorded with its time, so
the view says why there is a gap rather than leaving one.

The duplicate check is per schedule, identified by `schedules.id`: each job
can have one automatic run queued or running. Different schedules can each
queue one run in the same project. Their prompts wait behind active project
work and follow the project queue ([012](012-task-queue.md)). An idle project
starts its next queued prompt immediately.

Stopping a queued run closes its history entry as `interrupted`, without
spending the counter. Its schedule can queue another run at the next slot.
A prompt that cannot start closes its run as `error`, so the schedule is not
left busy forever. A provider outage keeps the prompt queued and the run open.

A skip that follows another skip extends it rather than adding a row of its
own: the view reads *skipped 12 times, 09:00–12:00* instead of twelve
identical lines saying the same thing every fifteen minutes. The count and the
time range live on the one row from the moment the second skip lands, so
nothing is collapsed after the fact — a schedule stuck behind one slow run
never has more than one skip row to show for it. A run finishing and the
schedule falling behind again starts the next skip, if there is one, as a
fresh row.

## Running one now

A schedule's view carries a Run menu beside Pause/Resume and Delete: the same
Send and Enqueue the prompt bar's own Send menu offers, minus Schedule — a
schedule cannot itself be scheduled. Whichever is chosen fires the schedule
outside its own clock, the way a tick does — a new session, a new
`schedule_runs` row, the prompt as the schedule stands — except in the two
ways forcing it implies:

- **It ignores the schedule being busy.** A tick due while the previous run is
  queued or running is skipped ([above](#skipping)); a forced run starts
  anyway, since each run is its own session and there is nothing
  for two to conflict over. A schedule can show more than one run in flight at
  once, the same as a project can.
- **It never spends the counter**, win or lose. This is what makes the button
  useful on a schedule that is paused or already at `0`: trying the prompt
  once costs nothing on either count, so neither state is a reason to disable
  it.

`next_run_at` is untouched either way — a run forced in between two ticks does
not move the next one, since nothing about the recurrence asked for it.

Send and Enqueue mean what they mean in the prompt bar ([012](012-task-queue.md)):
Send starts beside whatever else the project is running; Enqueue waits for the
project to be free, and until then the run is visible on its own new task, the
same dashed queued bubble any waiting prompt draws. Which one the split button
runs directly follows the same Enter binding the prompt bar reads
([017](017-general-settings.md)); the menu beside it offers the other by name.

## Pause, archive and the sidebar

A schedule row in the sidebar carries **Pause** / **Resume**. Paused, nothing is
scheduled at all until it is resumed, and resuming restarts the cadence from
now rather than firing everything the pause held back.

The archive icon appears **only on a paused schedule**. Archiving something that
is still firing would hide it while it kept starting tasks, which is the one
state nothing in the UI would explain. Archived therefore implies paused, and
unarchiving leaves it paused — a schedule comes back where it was put down, not
mid-burst.

**An archived job is not a lost one.** The sidebar holds what a project has
scheduled; the project page's **Jobs** tab holds that and the archive under it
— open jobs first, in the order the sidebar was dragged into, then the
archived ones, newest first and grey. That is the same list in the same two
colours the Tasks tab beside it draws ([030](030-project-tasks.md)), because
the question being asked is the same one: where is that job. The restore icon
on a grey row is the way back.

An archived row offers no Pause or Resume. Archived already means paused, and
resuming something put away would schedule nothing — so the row carries its
name, its clock, when it was archived, and the one control that changes any of
it.

Archived or not, a schedule stays a schedule. It is a row in `schedules`, so no
list has to guess whether something was a schedule or an ordinary prompt.

A job's row answers to `Ctrl+PageUp` and `Ctrl+PageDown` with every other row
in the sidebar, in the order the sidebar draws them — the project, then its
jobs, then its tasks ([004](004-ui.md#tabs)). A job is a row like any other and
is reached like one; the numbered `Alt` chords stay on tasks, which are what
gets typed into.

**Runs stay in the project's sidebar until the user archives them.** Finishing
a turn closes the run history entry and spends a run from the schedule's
counter, but leaves the task open, including when the turn fails. Runs started
manually also stay open and do not spend the counter. The schedule's own view
keeps every run in its history. Archiving a task by hand moves it to the grey
half of the project's Tasks tab ([007](007-task-closing.md)) and asks what it
came to ([051](051-task-outcomes.md)).

## The view

A schedule tab looks like a project page ([004](004-ui.md#tabs)), is its own
colour on the strip, and sits in the same **context slot** a project page and a
conversation share ([008](008-project-workspaces.md)): opening a job replaces
whichever of the three was in front rather than adding a tab beside it. A job
is read the way a page is, and walking a project's jobs with `Ctrl+PageDown`
would otherwise leave one tab behind per job looked at. Its label is cut to the
same 32 characters (including `...`) a task's is, since a job names itself from a prompt exactly
as a task does ([above](#the-name)). Its header carries the job's name and number
([above](#the-number)), the recurrence in words, when the next run is due, a
Run menu ([above](#running-one-now)), Pause/Resume and Delete.

**An archived job's header is the shelf it sits on.** It reads *archived* and
the date in place of the next run, and carries **Unarchive** where an open one
carries Run and Pause: nothing put away offers to start work, and the one
thing worth doing to it is bringing it back. What it repeats stays editable,
so a job whose prompt was the reason it was archived can be fixed before it
returns.
Under it sit the prompt, the model, the
**Repeats** fields and the runs-left input; and under those every run it has
spawned, newest first, each with its timestamp as `YYYY-MM-DD HH:mm:ss`.

**The prompt and the model are as editable as the clock.** The prompt is a
text box that commits when it is left, and the model is the shared picker with
its separate effort select. It includes ordered favourites but not the main
prompt's favourite button. Nothing about what a schedule repeats is a
decision made once: a typo, or a model that turned out to be the wrong one,
should not cost the runs the schedule has already recorded. Only future runs
follow the change — a run already spawned is an ordinary session and keeps the
prompt and model it was started with. The title is not one of them: it is
named once and renamed by hand after that ([above](#the-name)).
Switching to a model that has no such effort drops the effort rather than
sending one its provider would reject. A
spawned run is a task, so its row opens it. A skipped run is a row with no task
and the reason it was skipped; a run of consecutive skips is still one row, its
single timestamp widened to the range it spans and its reason naming how many
fires it stands for — *skipped 12 times*, not twelve identical lines.

Repeats is the dialog's own control less the number of runs: the recurrence,
then hours and minutes, or a time of day with day spacing or weekday choices.
A field commits when it is left, and the header's words and next run follow it — which is the confirmation that the
new clock took, without a Save button to press. Weekly and monthly forms say
that their day comes from the anchor. A weekday selection commits on each
click, and its final selected day cannot be unchecked. Both Repeats and Runs
left are only rebound to the schedule while nobody is typing in them: a fire
arriving mid-edit would otherwise take the caret with it.

A schedule stores only the half of the clock its form uses — the other is 0 —
so switching form in the view offers what the dialog would have offered, a
quarter of an hour or 09:00, rather than an interval of nothing or midnight
nobody asked for.

Those times are **local**, where task and project Stats timestamps are UTC.
The layout is the same; the zone is not, and deliberately: a schedule is read
off the clock on the wall, so "every day at 09:00" listing its runs at 07:00
would read as simply wrong. `isoLocal` sits beside `isoDate` in `api.js`
for that.

A schedule tab has no right-hand panel. Changed and Commits describe work in
the repository; the schedule page already carries everything its clock knows.

## Data

Schedules use two tables and link each spawned session through `schedule_id`:

- **schedules** — `id, project_id, title, prompt, provider, model, effort,
  permission, finish_condition, every, interval_minutes, at_minute, day_interval, weekday_mask,
  anchor_at, remaining, paused, next_run_at, created_at, done_at, position`.
  `every` is `interval`, `day`,
  `days`, `weekdays`, `week` or `month` for repeating jobs (`pinned` for manual-only
  prompts). `day_interval` defaults to 1; `weekday_mask` defaults to 0 and uses
  Sunday as bit 0 through Saturday as bit 6. Both fields are added through an
  append-only migration, so existing clocks keep their behavior. `at_minute`
  is minutes past server-local midnight for calendar forms; `interval_minutes`
  is the whole X hours Y minutes for the first. `anchor_at` is when the form
  was chosen — the creation time until
  the recurrence is edited into a different form — which is what fixes the
  N-day starting date, weekday and day of the month. Changing N-day spacing
  also moves this anchor. `finish_condition` is empty when the job has none.
  `done_at` and `position` mean what they mean on a
  session row: archived-at, and the order the sidebar was dragged into.
- **schedule_runs** — `id, schedule_id, session_id, status, started_at,
  ended_at, count`. `status` is `running`, `done`, `finished`, `error`,
  `interrupted` or `skipped`. `finished` is a run that found the finish
  condition true. A skipped run has no `session_id`. This is the list the view
  draws, and it is why skips can be shown at all: they never become sessions.

  `count` is how many fires one row stands for — 1 for everything except a
  run of skips, which grows the latest skip row's `count` and `ended_at`
  instead of inserting another: a schedule stuck behind one slow run reads as
  one line, a count and a time range, not one row per fire it waited out.

  `running` means the run is open, including while its prompt is queued or
  waiting for a provider retry. Startup preserves these open rows when a
  queued prompt still exists, so restarting cannot enqueue a duplicate.
  The schedule clock dispatches ready queued runs once at startup, without
  waiting for their next recurrence slot. Held provider retries keep their
  retry time and resume through the retry clock.
  Runs with no queued prompt become `interrupted` at startup: no provider
  process survives a restart. An interrupted run spends nothing.
- **sessions.schedule_id** — 0 for an ordinary session. It is what lets a
  finishing turn find the schedule that started it without a lookup per turn.
  It is the open **run row** that decides whether a run is spent, though, not
  this column: a scheduled task the user later prompts by hand has no open run,
  so it does not spend another run.

## Firing

The runner owns a ticker, started with the server and stopped with it. Every
15 seconds it asks the store for schedules that are due — `paused = 0 AND
done_at = 0 AND remaining <> 0 AND next_run_at <= now`, one index — and for
each one either creates a queued task or records a skip. It then dispatches
the affected projects through the ordinary queue. Enqueueing holds the project
transfer read lock; dispatch happens after releasing it, since starting a turn
also takes that lock. A tick that finds nothing costs one query, which is the
point of storing `next_run_at`.

Fifteen seconds is the resolution: a daily schedule set for 09:00 fires within
fifteen seconds of it. Minute-accurate to the eye, and cheap enough that the
idle cost of the feature is a query every quarter minute.

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/schedules` | `?window=&q=&project_id=&include_done=` — the sidebar list, and with `include_done` the project page's Jobs tab, which draws both states |
| POST | `/api/schedules` | `{project_id, prompt, provider, model, effort, permission, every, interval_minutes, at_minute, remaining, finish_condition, title}` — `title` and `finish_condition` optional; without it the job names itself ([above](#the-name)) |
| GET | `/api/schedules/:id` | the schedule and its runs |
| PATCH | `/api/schedules/:id` | `{title, prompt, finish_condition, provider, model, effort, remaining, paused, done, every, interval_minutes, at_minute}` — any subset |
| DELETE | `/api/schedules/:id` | drops the schedule and its run history; the sessions it spawned stay |
| POST | `/api/schedules/:id/run` | `{enqueue}` — force one run now; see [Running one now](#running-one-now) |

The three recurrence fields move together on a PATCH, because they are one
answer: `every` names the form and the other two carry it. `provider`, `model`
and `effort` move together for the same reason — an effort belongs to a model
and a model to a provider — and are read by the same check `POST` uses, so a
model can never name a provider the row has never heard of. They are read by
the same check `POST` uses, so the two cannot drift apart, and a rejected
clock changes nothing. Both a new clock and a resume book `next_run_at` once,
after whichever came in has been applied — a request carrying both gets one
booking, off the new recurrence.

`GET /api/projects/:id` also carries the project's open schedules, the way it
already carries its open tasks, so the Projects sidebar needs no second
request. Each of its task refs carries `schedule_id` with it, which is the
number a run draws; a sidebar that had to ask per row which job a task came
from would cost a request per run.
