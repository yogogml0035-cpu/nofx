---
name: trellis-finish-work
description: "Verify the current task and follow the project's local integration, test handoff, and archive steps."
---

# Finish Work

Locate the current task's worktree with `git worktree list`, then load the repository's delivery step from that worktree.

```bash
python3 ./.trellis/scripts/task.py current --source
python3 ./.trellis/scripts/get_context.py --mode phase --step 3.5
```

- Verify that the task's full checks passed and Phase 3.4 committed the task's changes.
- For a child, integrate and archive in the parent worktree, then clean up the child's local branch and worktree. The next child starts from the updated parent.
- For an independent task or parent, push the validated feature and complete the local `dev-offline` handoff. The user runs, pushes, deploys, and validates the test candidate.
- After the user confirms that the deployed test version passed, archive the root in its original feature worktree. Use `task.py archive` and `add_session.py` there, commit the records if needed, and push them to the same feature branch. The user handles production integration and cleanup.

Scope operations to the current task. See the Git integration and retention rules in `AGENTS.md`.
