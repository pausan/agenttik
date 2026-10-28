# Task flags

A task can carry one mark, like a mail client's stars:

| Flag     | Icon                              |
|----------|-----------------------------------|
| `star`   | filled yellow star                |
| `warn`   | amber warning triangle            |
| `danger` | red circled exclamation mark      |

The mark replaces the status dot on sidebar, project and archived task rows.
It pulses while the task runs, so a marked row still shows running work:
like the running dot it shrinks to 60% and grows back over two seconds, but
fades almost out (10% opacity) where the dot only halves.
Unread still makes the title bold; read and unread now live only in the
row's right-click menu.

Clicking the dot steps read → unread → star → warning → danger → read
without selecting the task. An unmarked task left unread by a finished turn
is only read by the first click, so the next click starts the cycle again.
A task marked unread by hand or marked skips the unread step and moves to
its next mark; clearing the last mark also marks it read. It works while the
task runs or has queued prompts. The
right-click menu has a **Mark** submenu that sets a mark directly; choosing
the checked one clears it.

The mark is stored in `sessions.flag` and sent as `flag` on session rows and
the sidebar's `SessionRef`. `PATCH /api/sessions/:id` with `{"flag": …}`
changes it; anything other than the three names or `""` is refused. Setting
a mark is not activity, so `last_active_at` stays put. The row updates at
once and reverts if the request fails.

Archiving and unarchiving keep the mark. The archived list sorts marked tasks
first, then newest first, in SQL, so the order holds across pages. Open task
lists keep their dragged order.

`task-flags.test.js` checks the cycle, the unread step and reading a
finished turn. `session_paging_test.go` checks the
archived order, unknown flags and the sidebar field. The browser test steps
a running task's dot through unread and the marks, reads a finished result
from its dot, checks a running mark
pulses and a stopped one does not, sets a mark from the menu,
reloads, and checks that a marked, unread archived task leads a newer
unmarked one with a bold title.
