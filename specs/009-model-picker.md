# Shared model picker

`ModelSelection.vue` is the model control in the main prompt, transcript
message editor, queued prompt, scheduled job and pinned prompt. It always
draws a model button and a separate effort select. Only the main prompt adds
the favourite button.

The model button and every model row use one line: `Subscription · Model`.
Rows sit under collapsible `Subscription · Provider` groups, so the provider
is visible without repeating it on every row. The fuzzy field matches the
subscription, provider display name and id, model display name and id.

Favourites come first, in their Settings order, as `Subscription · Model ·
Effort`. Their section is independently height-limited, and the whole palette
is bounded by the viewport. Models from an unavailable CLI remain visible but
disabled.

A favourite carries provider, subscription, model and effort. Picking it sets
the whole combination. Changing the ordinary model keeps the current effort
when the new model accepts it and otherwise returns to Default.

Settings can hide one subscription/provider group or one model in that group.
Hidden choices, including their favourites, disappear from every picker. A
task or job already using a hidden choice keeps it and still names it on the
closed button; visibility is a menu preference, not a change to saved work.

Choosing a model from another provider — or another subscription of the same
one — changes the task's provider and clears the provider-owned thread id. The next prompt starts a new thread on that
provider; opaque thread ids cannot be resumed across CLIs. The API validates
the selected provider, its availability, and that the model belongs to it.
