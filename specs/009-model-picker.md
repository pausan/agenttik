# Shared model picker

`ModelSelection.vue` is the model control in the main prompt, transcript
message editor, queued prompt, scheduled job and pinned prompt. It always
draws a model button and a separate effort select. The main prompt and the
message editor also draw the favourite button.

The closed button reads `Subscription · Model`, and its tooltip spells the
whole choice out for a name too long to fit.

Rows say only the model: the group above them already names the subscription
and the provider, so nothing is repeated per row. The fuzzy field still
carries the subscription, provider display name and id, and the model display
name and id, so searching a provider finds its models.

A model name drops a vendor prefix its own connection already says —
"Anthropic: Claude Sonnet 4.5" under an Anthropic connection is "Claude Sonnet
4.5", while the same name under OpenRouter keeps the vendor that tells its
rows apart. See `web/src/model-labels.js`.

Favourites come first, in their Settings order, as `Subscription · Provider ·
Model · Effort`. Their section is independently height-limited, and the whole
palette is bounded by the viewport. Unavailable CLIs, signed-out subscription
accounts, disabled APIs, and their favourites are omitted from settings and
every picker. API setup and discovery are described in
[078](078-api-providers.md).

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

Codex discovers its visible models and reasoning efforts through the installed
CLI's `app-server` `model/list` endpoint, following all pages in CLI order.
The provider-wide catalog uses the CLI's default login and configuration.
Results are cached for ten minutes and refreshed on the next catalog request
(for example, reloading the app). Discovery starts no conversation or model
turn and times out after ten seconds. Failed or empty replies retain the last
successful list; before any success, the bundled list is the fallback.
New model ids and effort levels need no app update. Context-window sizes are
unknown until turn usage reports them because this endpoint does not provide
those sizes. Saved model choices are not migrated.
