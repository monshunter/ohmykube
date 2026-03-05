---
name: tdd
description: "IMPORTANT: Invoke this skill automatically when implementing code following a plan checklist. Do NOT write implementation code against a checklist without invoking this skill first. Test-Driven Development workflow with strict checklist progression, immediate checklist sync, and document lifecycle coordination. Use when implementing features from implementation/unit-test/e2e/frontend checklists. Triggers on /tdd or when working against checklist-based plans."
---

# TDD Development Skill

Execute checklist-driven development with mandatory Red-Green-Refactor cycles.

## Usage

- `/tdd` - Use checklist already present in current context
- `/tdd --file <checklist>` - Explicit checklist path
- `/tdd --file <checklist> --references f1,f2,...` - Checklist + additional references
- `/tdd --file <checklist> --test-checklist <path> --references f1,f2,...` - With associated test checklist
- `/tdd --file <checklist> --section <prefix> --references f1,f2,...` - Section-scoped execution
- `/tdd -h` - Show help only, do not execute workflow
- `/tdd -h -v` - Show verbose help (including workflow), do not execute

## Argument Contract

- `--file <path>`: checklist markdown file
- `--test-checklist <path>`: associated test checklist with `<!-- phase-mapping: -->` annotations
- `--references <p1>,<p2>,...`: comma-separated markdown references (plan/spec/test-plan)
- `--section <prefix>`: Phase prefix (e.g., `W1.Auth`) to scope execution to a single checklist section

Rules:

- When arguments are provided, `--file` is required.
- Split `--references` by comma, trim whitespace, ignore empty segments.
- If a referenced file is missing, stop and report error before coding.

### `--section` mode

When `--section <prefix>` is provided, `/tdd` operates in **section-scoped mode**:

1. **Section matching**: Find the `## ` heading line that starts with `## {prefix}` (e.g., `## W1.Auth:`). Process only the checkbox items (`- [ ] ...` / `- [x] ...`) between that heading and the next `## ` heading (or end of file).
2. **Step 2 (lifecycle status) is skipped**. The caller (e.g., `/implement` lead agent) is responsible for lifecycle management.
3. **Step 3 (select next item)** only considers items within the matched section.
4. **Steps 4-8** are unchanged (Red-Green-Refactor + immediate checklist update).
5. **Step 10 (completion lifecycle sync) is replaced**: when all items in the section are checked, report `Section {prefix} complete` and stop. Do NOT trigger global lifecycle sync.
6. **Prohibited**: modifying checklist items outside the matched section.

## Workflow

### Step 0: Handle help flags

- `-h`/`--help`: show skill name, description, usage, then stop.
- `-h -v`/`--help --verbose`: show usage + full workflow, then stop.

### Step 1: Load working documents

**With arguments**:

1. Read checklist from `--file`.
2. Read each file from `--references`.
3. If `--test-checklist` is provided:
   a. Read the test checklist file.
   b. Parse all `<!-- phase-mapping: {id} -->` HTML comments following `## ` section headings.
   c. Build a mapping table: `{impl-phase-id} → [test-section-heading, ...]`.
   d. Log the mapping summary (e.g., "Phase 2 → test sections 1, 2, 14").

**Without arguments**:

1. Locate checklist from current context.
2. If multiple candidates exist, ask user to choose one.
3. Read the chosen checklist and existing context references.

### Step 2: Check plan/checklist lifecycle status

After loading documents:

1. Read Header `状态` from checklist and its associated plan.
2. If first implementation run and status is `draft`, ask user:
   > "Plan/checklist is `draft`. Switch to `active` now?"
3. If user approves:
   - Update Header `状态` to `active`.
   - Update `更新日期` to today (`YYYY-MM-DD`).
   - If Header field order/enum/date is non-compliant, invoke `/sync-doc-index --fix-header` first.
   - Invoke `/sync-doc-index --fix-index` to sync INDEX projection.

### Step 3: Select next checklist item (strict order)

1. Find next unchecked item in original checklist order.
2. Announce the exact item before coding:

```text
Executing: {checklist-file} -> {section/item-id} {item-title}
```

3. Never skip unchecked items.

### Step 4: Red phase (MANDATORY)

For the current checklist item, before touching non-test source files:

1. Determine target test file using project conventions (`*_test.go`, `*.test.ts`, etc.).
2. Add/adjust a test case for this item.
3. Run a focused test command and verify Red:
   - Go: `go test ./path/to/pkg -run TestName -count=1`
   - JS/TS: use repository-configured focused command
4. If test passes immediately, record explicit note:

```text
Red phase note: test already passes (pre-existing implementation). Continue with coverage adequacy check.
```

### Step 5: Green phase (minimal implementation)

1. Modify non-test source files only after Step 4.
2. Implement the minimal change required for current item.
3. Re-run focused test; it must pass.

### Step 6: Refactor phase

1. Refactor for readability/maintainability without behavior change.
2. Re-run focused tests after refactor.

### Step 7: Verification for current item

1. Run focused tests for the item.
2. Run adjacent/regression scope tests (same package/module) as needed.
3. If tests fail, item remains incomplete.

### Step 8: Update checklist immediately

After current item is verified green:

1. Mark the exact checklist checkbox as complete.
2. Save checklist changes immediately (no batch update).
3. Continue to next unchecked item.

### Step 9: Execute mapped test items (when --test-checklist provided)

When `--test-checklist` is provided and the current implementation phase is complete
(all items in the current `## N` or `## W{n}.{Phase}` section are checked):

1. Look up the phase-mapping table built in Step 1.
2. Collect all test checklist sections mapped to the just-completed implementation phase.
3. For each mapped test section, execute its unchecked items using the standard
   Red-Green-Refactor cycle (Steps 4–8), but updating the test checklist instead.
4. When all mapped test items are checked, continue to the next implementation phase.
5. If no test sections map to the current phase, skip this step.

Phase completion detection:

- Sequential mode: the implementation checklist is grouped by `## N` sections (e.g., `## 1 Phase 1`, `## 2 Phase 2`). When the last unchecked item in a section is marked complete, Step 9 triggers.
- Parallel mode (`--section`): the section is identified by the `--section` prefix (e.g., `W1.Auth`). The phase-mapping uses Wave/Phase IDs: `<!-- phase-mapping: W1.Auth -->`.
- Test checklist items reference the `<!-- phase-mapping: {id} -->` annotation immediately following each `## ` heading in the test checklist.

### Step 10: Completion lifecycle sync

When all checklist items are checked:

1. If `--test-checklist` was provided, verify all mapped test checklist sections are also fully checked. If any mapped items remain unchecked, report the gap and continue executing them via Step 9 before proceeding.
2. Ask user:
   > "All checklist items are complete. Switch plan/checklist to `completed` and sync INDEX?"
3. If approved:
   - Set Header `状态` to `completed` on plan and checklist.
   - Update `更新日期` to today (`YYYY-MM-DD`).
   - If Header field order/enum/date is non-compliant, invoke `/sync-doc-index --fix-header` first.
   - Invoke `/sync-doc-index --fix-index` to sync INDEX grouping.

## Prohibited Actions

- Writing non-test implementation code before the corresponding Red test
- Skipping checklist items or changing execution order without approval
- Batch-checking multiple checklist items after the fact
- Claiming "tests passed" without actually running tests
- Modifying plan semantics/checklist scope without user approval
- In `--section` mode: modifying checklist items outside the matched section scope

## Plan Change Protocol

If plan/checklist content is wrong or outdated:

1. Stop current coding.
2. Explain the mismatch and proposed change.
3. Wait for explicit user approval.
4. Resume only after decision is confirmed.

## Bug Fix Protocol

When the checklist item is a bug fix:

1. Add a reproducer test first (Red).
2. Verify reproducer fails.
3. Apply fix (Green).
4. Verify test passes.
5. Evaluate and invoke `/bug-report` if bug knowledge entry is needed (mandatory project protocol after bug fix).

## Test Completeness Requirements

- Every checklist item has at least one corresponding test assertion.
- Normal path, boundary path, and error path are covered where applicable.
- Checklist completion must be backed by passing test evidence.
- Do not invent extra coverage thresholds unless the plan explicitly requires them.
