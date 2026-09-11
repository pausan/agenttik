# Agents Rules


## Rules

1. **Think before coding.** State assumptions and tradeoffs. If a choice materially changes the work, ask. Don't overcomplicate.
2. **Simplicity first.** Minimum code that works. No speculative features or abstractions unless explicitly requested.
3. **Surgical changes.** Touch only what is needed. Match existing style. Remove dead code.
4. **Goal-driven.** Define a verifiable success criterion (a test, a measured number, a screenshot) and loop until it passes. Give a short plan for multi-step work.
5. **Plain language.** Short sentences, common words, concrete statements.
6. **Tests guard regressions.** ~~Create unit tests whenever possible and reasonable and run them regularly.~~ Since we are currently prototyping, don't create or run tests unless explicitly asked.
7. **Performance matters.** Memory and CPU cycles are not free. Let's design
with this in mind. Fast is a feature.
8. **Document in `specs/`.** Technical details, current behaviour and status. Describe the design as it stands, not the ones it replaced — when behaviour changes, rewrite the spec rather than appending to it. Keep an index file, keep index short, keep files short and organized.
9. **Commits.** Commit each self-contained change as you finish it, without asking. Small, regular, descriptive. No AI tool mentions in commit messages — a `commit-msg` hook rejects such trailers, install it once per clone with `make hooks`. Never push.