# Skill: Dangerous ops (data loss / prod breakage guardrail)

- **No destructive commands** (delete, migrate, format, overwrite, `rm -rf`, DROP TABLE, etc.) **without an explicit backup/rollback note** in `memory-bank/current_task.md`.
- Before running anything that can destroy data or change production state: document what you will do, how to undo it, and (if applicable) where the backup is.
- If a tool or suggestion says "just run this" and it is destructive, stop and add the note first.

Agents will happily run destructive commands if a tool suggests it. This rule prevents silent catastrophe.
