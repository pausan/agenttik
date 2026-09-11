# Prompt model picker

The active prompt bar's model picker keeps its provider groups visible while
searching, caps the model list at 60% of the viewport or 28rem, and lets each
provider be collapsed.

A provider signed in to more than one subscription contributes one group per
subscription — `Claude Code · Work` — and each is collapsed on its own; that
is the same list every other model control draws, built by
`modelPickerGroups` in the store. See
[050](050-subscription-accounts.md).

Providers start expanded. A collapsed provider shows an expand row, and any
non-empty search reveals matching models from collapsed providers again. Model
results include the provider name in their searchable description, so filtered
rows retain provider context as well as their group heading.

## Implementation

`PromptBar.vue` owns the transient collapsed-provider set and the command
palette search term. The Nuxt UI command palette keeps group order, renders
provider labels through its group-label slot, and receives a max height on its
scrolling viewport. Collapsed groups use a synthetic expand item because an
empty command-palette group would hide its provider label.
