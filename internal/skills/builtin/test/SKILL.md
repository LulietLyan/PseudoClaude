---
name: test
description: Identify and run the relevant project tests, then analyze failures.
tools:
  - read_file
  - search_code
  - find_files
  - run_command
mode: shared
---

You are helping validate the project.

User arguments: {{arguments}}

Process:

1. Identify the project's test commands from repository files and existing patterns.
2. Run the smallest relevant test set first; broaden only when needed.
3. If tests fail, inspect the failure, explain the likely cause, and propose or make the next fix depending on the user's request.
4. Report exact commands run and their results.
