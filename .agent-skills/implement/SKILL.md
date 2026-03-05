---
name: implement
description: "IMPORTANT: Invoke this skill automatically when the user asks to implement an existing plan. Do NOT implement a plan without invoking this skill first. End-to-end plan implementation entry point: resolves plan + target, validates docs/plan/<name>/context.yaml with bundled scripts, loads plan context, then mandatorily hands off checklist execution to /tdd. Triggers on /implement or when user says 'implement plan', 'execute plan', 'start implementing'. Supports /implement <plan-name> [target] syntax."
---

# Implement Skill

End-to-end plan implementation entry point. Resolve plan + target, validate `context.yaml`, load all referenced documents, then invoke `/tdd` for checklist-driven coding.

## Usage

- `/implement` - List latest candidate plans and let user choose
- `/implement <plan-name>` - Implement default target of the named plan
- `/implement <plan-name> <target>` - Implement a specific target (for example `frontend`, `unit-test`)
- `/implement -h` - Show help only, do not execute workflow
- `/implement -h -v` - Show verbose help (including full workflow), do not execute

## Prerequisites

- Plan directory exists under `docs/plan/<name>/`
- Manifest exists at `docs/plan/<name>/context.yaml`
- `python3` is available and can run bundled scripts
- `PyYAML` is installed (required by validator and candidate scripts)

## Workflow

### Step 0: Handle help flags

- If user passes `-h` or `--help`, show: skill name, description, usage.
- If user passes `-h -v` or `--help --verbose`, also show full workflow.
- In both cases, stop after help output.

### Step 1: Resolve plan name

**With argument** (`/implement local-auth`):

1. Check if `docs/plan/{name}/` exists.
2. If not found, read `docs/plan/INDEX.md` and fuzzy-match plan names.
3. If multiple matches, list candidates and ask user to choose.
4. If no match, stop and report available plan names.

**Without argument** (`/implement`):

1. Run:

```bash
python3 .agent-skills/implement/scripts/list_context_candidates.py \
  --plan-index docs/plan/INDEX.md \
  --plan-root docs/plan
```

2. Display numbered candidates with reasons.
3. Ask user to select one number.
4. If no candidates, show script output and stop.
5. If input is invalid, re-display list and wait (do not proceed).

### Step 2: Read manifest and determine target scope

1. Read `docs/plan/{name}/context.yaml`.
2. Determine target:
   - If user passed `<target>`, use it.
   - Otherwise use `spec.defaultTarget`.
3. If target is not defined in `spec.targets`, stop and show available targets.

### Step 3: Validate manifest and collect normalized file set

Run validator for the selected target:

```bash
python3 .agent-skills/implement/scripts/validate_context.py \
  --context docs/plan/{name}/context.yaml \
  --docs-root docs \
  --target {target}
```

Expected behavior:

- Exit code `0`: stdout is normalized JSON with `files[]`.
- Non-zero: print stderr and stop.

| Exit code | Meaning |
|-----------|---------|
| 0 | Validation passed |
| 2 | Schema/field validation failed |
| 3 | Declared file does not exist |
| 4 | Path escapes `docs/` boundary |
| 5 | Target does not exist |

From validator output:

- Locate exactly one `role == "checklist"` path as `/tdd --file` input.
- If a `role == "test-checklist"` file exists, extract its path as `/tdd --test-checklist` input.
- If a `role == "test-plan"` file exists, include it in `/tdd --references`.
- Treat all other validated files (except checklist and test-checklist) as `/tdd --references` inputs.
- Keep file order stable and deduplicate by absolute path.

### Step 4: Load context and present summary

Read every validated file from Step 3, then summarize to user:

1. Plan name, selected target, and default target.
2. Loaded file list grouped by role (`plan`, `checklist`, `spec`, ...).
3. Checklist progress (checked/total).
4. Plan Header lifecycle status (`draft`, `active`, ...).
5. Whether the plan doc contains Wave/Phase parallel structure.

### Step 5: Determine execution mode

Decide whether to use parallel (Agent Teams) or sequential execution.

#### 5.1 Check parallel eligibility

All of the following must be true for parallel mode:

1. Plan Header `执行模式` is `parallel`.
2. Plan document contains structured DAG metadata in HTML comments under `### W{n}.{Phase}:` headings:
   - `<!-- agent: {role} -->` — role ID from `AGENTS.md` §5.1
   - `<!-- depends-on: {deps} -->` — comma-separated Phase prefixes (e.g., `W1.Provider, W1.Config`)
3. Runtime has `TeamCreate` and `Task` tools available.

If any condition is not met → **sequential path** (Step 6A). Report the reason to the user.

#### 5.2 Parse DAG

When parallel eligible:

1. Find all `### W{n}.{Phase}: ...` headings in the plan document.
2. For each heading, extract HTML comments: `agent`, `depends-on`.
3. Group phases by Wave number (e.g., `W1` = `[W1.Provider, W1.Config]`).
4. Build dependency edges from `depends-on` values.

#### 5.3 Determine current Wave

1. Read the checklist and identify which phases have all items checked (complete) vs. have unchecked items (incomplete).
2. Find the earliest Wave that has incomplete phases.
3. Skip Waves where all phases are complete.

#### 5.4 Choose execution path

- If current Wave has **2+ incomplete phases** with different `agent` roles → **parallel path** (Step 6B).
- If current Wave has **1 incomplete phase** → **sequential path** (Step 6A) with `--section`.
- If current Wave has **0 incomplete phases** → advance to next Wave and repeat.

### Step 6: Execute

#### Step 6A: Sequential path

Invoke `/tdd` directly (same as previous Step 5 behavior):

```text
/tdd --file {checklist-path} --references {ref1},{ref2},...
```

If a `test-checklist` was found in Step 3, add `--test-checklist`:

```text
/tdd --file {checklist-path} --test-checklist {test-checklist-path} --references {ref1},{ref2},...
```

If a specific section was identified (single incomplete phase in a Wave), use:

```text
/tdd --file {checklist-path} --test-checklist {test-checklist-path} --section {phase-prefix} --references {ref1},{ref2},...
```

Rules:

- `--file` uses the validated checklist path only.
- `--test-checklist` uses the validated test-checklist path (if present).
- `--references` includes all other validated markdown files (including test-plan if present).
- Never write implementation code against checklist items before `/tdd` takes over.

#### Step 6B: Parallel path (Agent Teams)

Execute Waves in topological order. For each Wave with multiple incomplete phases:

##### 6B.1 Create team

```
TeamCreate: team_name = "{plan-name}-{wave}", description = "Parallel execution of {wave}"
```

##### 6B.2 Create tasks and spawn teammates

For each incomplete phase in the current Wave:

1. **TaskCreate** — subject: `{phase-prefix}: {phase-description}`, with details about the phase scope.

2. **Task** — spawn a teammate agent:
   - `subagent_type`: `general-purpose`
   - `team_name`: the team name from 6B.1
   - `name`: the `agent` role ID (e.g., `provider`, `initializer`, `config`)
   - `mode`: `bypassPermissions`
   - `prompt`: Include the following in the teammate prompt:
     - The checklist file path, references, and section prefix
     - Instruction to invoke `/tdd --file {checklist} --section {phase-prefix} --references {refs}`
     - The plan document content for the relevant section (so the teammate has full context)
     - File ownership boundaries from `AGENTS.md` §5.1
   - `run_in_background`: `true` (to allow parallel spawning)

3. Spawn all teammates for the same Wave **in a single message** (parallel tool calls).

##### 6B.3 Wait for completion

1. Monitor teammates via idle notifications and SendMessage.
2. Track progress via TaskUpdate status changes.
3. When a teammate reports "Section {prefix} complete", mark its task as completed.
4. Wait until all tasks in the Wave are completed.

##### 6B.4 Verify Wave completion

1. Read the checklist file.
2. Confirm all items in all phases of the current Wave are checked.
3. If any items remain unchecked, report the gap and stop.

##### 6B.5 Advance to next Wave

1. Shutdown teammates: SendMessage `shutdown_request` to all.
2. TeamDelete to clean up.
3. Repeat from Step 5.3 for the next Wave.
4. If next Wave has only 1 incomplete phase, use sequential path (Step 6A with `--section`).

#### Parallel path lifecycle management

- **Step 2 (lifecycle status)**: Handled by the lead agent before dispatching (unchanged from current behavior).
- **Step 9 (completion sync)**: Handled by the lead agent after all Waves complete. Teammates never trigger lifecycle sync — `/tdd --section` skips Step 2 and Step 9.

### Step 7: Completion check

After all Waves are executed (whether sequential or parallel):

1. Read the checklist and confirm all items are checked.
2. Confirm tests were actually run and passed.
3. If blockers remain, report unresolved checklist items and next required Wave/Phase.
4. If all items complete, proceed with `/tdd` Step 9 lifecycle sync (ask user to mark plan as `completed`).

## Error Templates

### Missing context.yaml

```text
ERROR: docs/plan/{name}/context.yaml not found.
Create a context.yaml manifest to use /implement. See docs/plan/README.md for template.
```

### defaultTarget not in targets

```text
ERROR: defaultTarget '{target}' is not defined in spec.targets.
Available targets: {list of target keys}
```

### Declared file not found

```text
ERROR: Referenced file does not exist:
  - {path1}
  - {path2}
Fix the paths in context.yaml and retry.
```

### Non-markdown file declared

```text
ERROR: Referenced file must be markdown (*.md):
  - {path} resolves to {resolved}
Fix context.yaml so plan/checklist/references are markdown documents.
```

### Path escapes docs/ boundary

```text
ERROR: Path escapes docs/ boundary:
  - {path} resolves to {resolved} which is outside docs/
Fix the relative paths in context.yaml.
```

### Target not found

```text
ERROR: Target '{target}' not found.
Available targets: {list of target keys}
Usage: /implement {name} <target>
```

### No candidates (no-arg mode)

```text
No plans with context.yaml found in Active/Draft status.
To make a plan available for /implement:
1. Ensure the plan is registered in docs/plan/INDEX.md
2. Create docs/plan/{name}/context.yaml (see docs/plan/README.md for template)
```

### Invalid candidate number

```text
Invalid selection. Please enter a number from the list above.
```
