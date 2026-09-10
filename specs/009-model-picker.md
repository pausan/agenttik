# Cross-provider model picker

The prompt bar's model picker lists every registered provider's models, with
favourites first even when they belong to a different provider. Models from an
unavailable CLI remain visible but disabled.

Choosing a model from another provider changes the task's provider and clears
the provider-owned thread id. The next prompt starts a new thread on that
provider; opaque thread ids cannot be resumed across CLIs. The API validates
the selected provider, its availability, and that the model belongs to it.
