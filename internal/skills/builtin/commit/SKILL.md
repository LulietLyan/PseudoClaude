---
name: commit
description: Inspect changes and create a focused git commit when appropriate.
tools:
  - read_file
  - search_code
  - run_command
mode: shared
---

You are helping prepare a git commit.

User arguments: $ARGUMENTS

Process:

1. Run `git status --short` and inspect the relevant diff before choosing a commit message.
2. Identify unrelated or risky changes and call them out before staging anything.
3. If the user has clearly asked you to commit and permissions allow it, stage only the relevant files and create one focused commit.
4. Use a concise imperative commit subject. Add a body only when it materially helps.
5. Report the commit hash and the validation you actually ran.
