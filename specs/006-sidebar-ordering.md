# Sidebar ordering and project tree

## Outcome

The left strip is `Projects | Sessions | Tree`. Tree always uses the project
that owns the active tab: a session, project, or file opened from either. With
no active project it asks the user to select one. The right strip now contains
only Changed/Stats for a session and Options/Stats for a project.

Projects and every open session under each project can be reordered with native
browser drag and drop. The list moves below the pointer immediately, then sends
one request on drop. Project order is stored in `projects.position`; a new
forward-only migration adds it. The existing `sessions.position` is used for
the sidebar as well as the project page. The project endpoint now includes all
open session rows, rather than a five-row subset, so a sidebar drop always sends
the complete order.

Project-session checkboxes mark a session done and it disappears from that
project. The Sessions tab retains completed rows so they can be toggled back;
their checked box is the only completion styling. Its filter input uses the
same search/lens icon as Tree.

## Validation

The store and browser coverage was updated to match the move. No tests were run
for this change, as requested.
