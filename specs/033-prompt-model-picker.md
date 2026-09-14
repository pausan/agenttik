# Prompt model picker

Every shared model picker keeps its subscription/provider groups visible while
searching, caps the list at 65% of the viewport or 30rem, independently caps a
long Favourites group at 11rem, and lets each group be collapsed.

A provider signed in to more than one subscription contributes one group per
subscription — `Work · Claude Code` — and each is collapsed on its own; the
rows beneath read `Work · Opus`. The list is built by
`modelPickerGroups` in the store. See
[050](050-subscription-accounts.md).

Providers start expanded. A collapsed provider shows an expand row, and any
non-empty search reveals matching models from collapsed providers again. Model
results search a separate subscription/provider/model string that is not drawn
as a redundant second line.

## Implementation

`ModelSelection.vue` owns the transient collapsed-group set and command-palette
search term. The Nuxt UI command palette keeps group order and renders group
labels through its group-label slot. Collapsed groups use a synthetic expand
item because an empty command-palette group would hide its label. A leading
check marks the current row; the closed button always shows the selected
subscription and model.
