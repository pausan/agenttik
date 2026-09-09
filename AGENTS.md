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
8. **Document in `specs/`.** Technical details, approaches tried and their outcome, and status. Keep an index file, keep index short, keep files short and organized.
9. **Commits.** Small, regular, descriptive. No AI tool mentions in commit messages.