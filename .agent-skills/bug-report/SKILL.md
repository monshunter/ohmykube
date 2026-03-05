---
name: bug-report
description: "IMPORTANT: Invoke this skill automatically after fixing a bug to evaluate whether a bug record is needed. Create structured Bug knowledge base records from investigation findings or interactive collection. Triggers on /bug-report."
---

# Bug Report Skill

Creates a structured Bug record in `docs/bugs/`, updates the Bug index, and optionally updates the pattern library.

## Trigger Command

```
/bug-report                              # Interactive mode
/bug-report --this                       # Organize from current session materials
/bug-report --file path/to/investigation.json   # Import from artifact
/bug-report -f path/to/investigation.json   # Import from artifact
/bug-report -f path/to/any-data-or-related-files  # Import from artifact
```

## Workflow

### Step 1: Read specification

Read `docs/bugs/README.md` to understand the current Bug record conventions (module enum, severity, status constraints, template, sanitization rules).

### Step 2: Allocate Bug ID

1. Read `docs/bugs/INDEX.md` to find the highest existing BUG ID (`BUG-NNNN`)
2. Calculate next ID = max ID + 1 (format: four-digit zero-padded, e.g., `0001`)
3. If there is no existing BUG row, start from `0001`
4. **Before writing**, verify that `docs/bugs/BUG-NNNN.md` does not already exist (conflict prevention)

### Step 3: Collect Bug information

Attempt to load information from three sources, in priority order:

**Source A — Investigation artifact** (`investigation.json`):

If `investigation.json` exists (current directory or user-provided path), normalize and extract:
- Normalize payload first:
  - If top-level key `investigation` exists and is an object, use `investigation.*`
  - Otherwise, use top-level fields directly (compatibility fallback)
- Field mapping:
  - `failure_symptom` → Symptom section
  - `investigation_steps` → Diagnosis Process section
  - `root_cause` → Root Cause Analysis section
  - `recommendation` → Fix section
- For `investigation_steps`, prefer `findings` as the main text and include `priority/area` when available

**Source B — Session context**:

If no `investigation.json` is available, or the artifact is missing required fields, collect interactively from the user:
- One-line title
- Module (must be enum: `controller` / `api-server` / `webhook` / `cli` / `ui` / `schema` / `infra` / `test`)
- Severity (`critical` / `high` / `medium` / `low`)
- Status (`open` / `investigating` / `resolved`)
- Symptom description
- Diagnosis process
- Root cause analysis (if resolved)
- Fix details (if resolved)
- Verification details (if resolved)
- Related commit title (if resolved), e.g. `fix(api-server): subject (BUG-NNNN)`

**Validation gates before writing**:
- Module value must exactly match enum values: `controller` / `api-server` / `webhook` / `cli` / `ui` / `schema` / `infra` / `test`
- Explicitly reject display names such as `API Server`, `Web UI`, `Controller`, and ask for enum value
- Status must be one of `open` / `investigating` / `resolved`

### Step 4: Create Bug record

Create `docs/bugs/BUG-NNNN.md` using the template from `docs/bugs/README.md`.

**Status constraints** — if status is `resolved`:
- "Root Cause Analysis" section must be filled
- "Fix" section must be filled
- "Verification" section must be filled
- Related commit field must not be `-` (use commit title, not SHA)

### Step 5: Update Bug index

Add a new row to the appropriate module table in `docs/bugs/INDEX.md`:

```markdown
| [BUG-NNNN](./BUG-NNNN.md) | title | severity | status | YYYY-MM-DD | `type(scope): subject` or `-` |
```

**Module-to-table mapping** (use the table section heading):
- `controller` → Controller
- `api-server` → API Server
- `webhook` → Webhook
- `cli` → CLI
- `ui` → Web UI
- `schema` → Schema
- `infra` → Infra
- `test` → Test

### Step 6: Update pattern library (optional)

If the Bug reveals a generalizable pattern:
1. Read `docs/bugs/PATTERNS.md`
2. Add a new pattern entry with the related BUG ID, typical symptoms, and check list
3. Ask the user to confirm before writing

If no generalizable pattern, skip this step.

### Step 7: Linkage prompts

Remind the user to:
- Reference the Bug ID in the work-journal entry: `See [BUG-NNNN](../bugs/BUG-NNNN.md)`
- Optionally include Bug ID in commit messages: `fix(scope): subject (BUG-NNNN)`

## Sanitization Checklist

Before writing the Bug record, verify:

- [ ] No real tokens, passwords, private keys, cookies, or full connection strings
- [ ] Sensitive values masked (e.g., `ghp_***`, `postgres://user:***@host/db`)
- [ ] Log/command output trimmed to minimum needed for diagnosis

## Investigation Artifact Contract

The `investigation.json` format (produced by `/test-investigate`):

```json
{
  "investigation": {
    "scenario_id": "L1.003",
    "run_id": "20260120-153045-a1b2",
    "investigated_at": "2026-01-20T16:00:00Z",
    "failure_symptom": "Description of what failed",
    "investigation_steps": [
      {
        "priority": 1,
        "area": "test",
        "findings": "Summary of test-level findings"
      }
    ],
    "root_cause": "Identified root cause",
    "recommendation": "Suggested fix"
  }
}
```

Compatibility note: this skill supports both nested format (`investigation.failure_symptom`) and flat format (`failure_symptom`) for artifact import.

**Orthogonality constraint**: `/bug-report` does not depend on `/test-investigate` being executed. `/test-investigate` does not drive the Bug record creation workflow.

## Bug Report Checklist

- [ ] Read `docs/bugs/README.md` specification
- [ ] Allocate Bug ID from INDEX.md (verify no conflict)
- [ ] Collect Bug information (artifact import or interactive)
- [ ] Validate module field uses enum value (not display name)
- [ ] Reject display names for module field (`API Server`, `Web UI`, etc.)
- [ ] Validate status constraints (resolved requires root cause/fix/verification/Related commit title)
- [ ] Run sanitization check (no secrets in record)
- [ ] Create `docs/bugs/BUG-NNNN.md`
- [ ] Update `docs/bugs/INDEX.md` with new entry
- [ ] Optionally update `docs/bugs/PATTERNS.md`
- [ ] Prompt user for work-journal and commit message linkage
