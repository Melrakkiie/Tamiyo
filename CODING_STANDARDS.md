# Coding standards

Review rules for this repo, on top of the rules in `CLAUDE.md`. Tooling already enforces formatting, lint (`.golangci.yml`) and delivered-migration immutability; skip those.

- **Comments:** names and tests carry the meaning; a comment only states a constraint the code can't show. SQL migrations keep their header comment explaining the change.
- **Dead code:** a change removes the code it leaves unused: functions, types, fakes, test helpers, and files that end up holding something unrelated to their name.
- **Shared test helpers:** a new fixture or helper reuses the package's existing one when it covers the case, instead of adding a near-duplicate.
