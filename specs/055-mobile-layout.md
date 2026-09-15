# Mobile layout

Below 768 CSS pixels, the app shows one full-width working pane. At 768px and
above, tablets and computers use the resizable desktop grid and its saved
panel widths. Resizing across the breakpoint closes any mobile drawer without
changing the selected project, task drafts, or saved desktop widths.

## Navigation

The phone header shows the current project and a **New task** button. With no
project selected, that button is **Add project**.

- The menu button opens **Projects and tasks** from the left. It contains the
  same projects, tasks, Tree, Add project, and Settings controls as the desktop
  sidebar. Choosing a project, task or job dismisses it, including reselecting
  the current row.
- The workspace button opens the right panel on demand. Projects offer Project Options,
  Changed and Commits; tasks offer Changed and Commits. Scheduled jobs have no workspace
  button. Repository selection remains available for multi-repository projects.
- Drawers cover the working pane, trap focus, and close through their close
  button, the backdrop, or Escape. Dismissal restores focus to the trigger.
  A selection that changes the active file tab also dismisses the drawer.

The tab strip scrolls horizontally and keeps task status and Stats available.
Model and effort badges and duplicate New task buttons are hidden on phones.
Task titles can wrap onto two lines, and row controls have larger touch areas.
Settings sections form a horizontal scrolling strip above the selected pane.

## Prompt and viewport

The phone composer has two control rows: model, effort and favourite above;
usage, Stop while running, and the send menu below. Existing Send, Enqueue and
Schedule behaviour is shared with desktop. Keyboard hints are hidden. Model
popovers fit the screen, and usage details stack vertically and scroll.
Transcript copy and edit controls remain visible without hovering.

Navigation does not automatically focus a prompt or filter on phones; tapping
a field starts editing. Desktop focus behaviour is unchanged. Phone fields
use 16px text to avoid focus zoom. Dynamic viewport height, the visual viewport
resize event, and safe-area padding keep the composer within the available
screen as browser chrome and the software keyboard change its height.

## Verification

`e2e/tests/mobile.spec.js` exercises touch project switching, task creation,
sending and stopping, draft preservation, drawer dismissal and focus, file
opening, adding a project, and Settings. It checks controls and popovers at
320, 390 and 767px, with a shortened viewport, and checks the desktop panels
and saved widths at 768, 1024 and 1440px. Browser emulation does not test a real
iOS or Android software keyboard.
