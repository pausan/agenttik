# Prompt model picker

Every shared model picker caps its list at 30rem or 62% of the visual
viewport, independently caps a long Favourites group at 11rem, and lets each
subscription group be collapsed.

A provider signed in to more than one subscription contributes one group per
subscription — `Work · Claude Code` — and each is collapsed on its own; the
rows beneath read `Opus`. API connections are one group named after the
service. The list is built by `modelPickerGroups` in the store. See
[050](050-subscription-accounts.md).

## Collapsing

With favourites saved, every subscription group starts collapsed, so the
favourites are not buried under every model of every provider. With no
favourites, the groups start open as before.

A collapsed group is a single row — `> Subscription · Provider` — with the
number of models it holds, or a check when the current model is one of them.
Choosing that row expands the group into its heading and its model rows;
choosing the heading collapses it again. Expansions last while the popover is
open and are forgotten when it closes.

Any non-empty search ignores collapsing entirely and shows every matching
model under its group heading.

## Implementation

`ModelSelection.vue` owns the transient per-group expansion map and the
command-palette search term. Only whether a search is running reaches the
group list, so typing never rebuilds it; the search options and the palette
`ui` object are module constants for the same reason, since a fresh object
would rebuild the palette's index on every keystroke. `App.vue` publishes the
visual-viewport height as `--mobile-height` on the document element, which is
what bounds the palette when a software keyboard is open — popovers are
teleported out of the app shell and cannot inherit it from there.

The Nuxt UI command palette keeps group order and renders group labels through
its group-label slot. An expanded group's heading is a button in that slot. A
collapsed group instead carries no label and names a `collapsed` slot that
does not exist, which is what stops the palette from drawing an empty heading
above its single row; an empty group would be dropped altogether, so the row
is what keeps the group alive. A leading check marks the current row.

`e2e/tests/model-picker.spec.js` covers the favourite heading format,
collapsing and expanding, search reaching into a collapsed group, vendor
compaction, and favouriting from the message editor.
