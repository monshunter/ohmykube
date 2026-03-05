---
name: sync-doc-index
description: "Check and repair document Header / INDEX drift. Validates Header field compliance (order, status enum, dates) and INDEX projection consistency across docs/spec/ and docs/plan/. Supports --check (audit), --fix-header (normalize Headers), and --fix-index (sync INDEX to Headers). Triggers on /sync-doc-index or when checking/repairing document Header/INDEX drift."
---

# Sync Doc Index Skill

Checks and repairs document Header / INDEX drift across `docs/spec/` and `docs/plan/`.

**Architecture**: Deterministic checks and auto-fixes are handled by `scripts/sync-doc-index.py` (bundled in this skill). The LLM only intervenes for judgment items the script cannot resolve automatically.

## Concepts

### Truth Source Model

| Layer | Truth Source | Purpose |
|-------|-------------|---------|
| Task completion | Checklist checkboxes | Whether implementation tasks are done |
| Document lifecycle | Header `状态` field | Whether document is draft/active/completed etc. |
| Directory display | `INDEX.md` | Read-only projection view, never a status source |

### Standard Header Contract

**Spec documents** (`docs/spec/*.md`, excluding `README.md` and `INDEX.md`):

```
> **版本**: X.Y
> **状态**: draft|active|completed|superseded|deprecated
> **更新日期**: YYYY-MM-DD
```

**Plan documents** (`docs/plan/*/*.md`, excluding `README.md`):

```
> **版本**: X.Y
> **状态**: draft|active|completed|superseded|deprecated
> **更新日期**: YYYY-MM-DD
> **执行模式**: parallel|sequential
```

Field order is **fixed**. Fields must appear in exactly this order.

**Checklist documents** (`*-checklist.md`): same as spec (3 fields, no `执行模式`).

### Status Enum

Valid values: `draft`, `active`, `completed`, `superseded`, `deprecated`

Legacy value mapping:

| Legacy | Normalized |
|--------|-----------|
| `实施中` | `active` |
| `已完成` | `completed` |
| `废弃` | `deprecated` |

## Modes

| Mode | Argument | Effect |
|------|----------|--------|
| Check | `--check` | Read-only audit; outputs report |
| Fix Header | `--fix-header` | Repairs document Headers in-place |
| Fix Index | `--fix-index` | Rewrites INDEX files to match Headers |

If no argument is provided, default to `--check`.

## Workflow: `--check`

1. Run:
   ```bash
   python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --check
   ```
2. Display the output to the user as-is.

## Workflow: `--fix-header`

### Step 1: Preview auto-fixes

Run:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-header --dry-run
```

Output has 3 sections: **Applied** (what will be modified), **Skipped** (auto-fix attempted but needs LLM), **Post-fix Verification** (full re-check result).

Show the preview to the user. If the user approves, proceed to Step 2.

### Step 2: Apply auto-fixes

Run:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-header
```

Review the output. If Post-fix Verification shows zero issues, done. Otherwise proceed to Step 3.

### Step 3: Handle remaining issues

The **Skipped** and **Post-fix Verification** sections in the output above list all items that need LLM intervention. For each:

- **Skipped items** (auto-fix couldn't complete): Read the file, determine the correct value from context, use Edit tool.
- **Missing `状态`/`版本`**: Read the document and its INDEX entry. Recover values from INDEX context. Use Edit tool.
- **No header at all**: Construct a complete header block using INDEX values and `git log` for date. Use Edit tool.
- **`header_not_adjacent`**: Read the file, verify the header block is correct, move it to immediately after the title line if needed.

If there are many remaining items, run `--check --json` for machine-readable output:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --check --json
```

## Workflow: `--fix-index`

### Step 1: Preview auto-fixes

Run:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-index --dry-run
```

Output has 3 sections: **Applied**, **Skipped**, **Post-fix Verification**.

Show the preview to the user. If the user approves, proceed to Step 2.

### Step 2: Apply auto-fixes

Run:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-index
```

Review the output. If Post-fix Verification shows zero issues, done. Otherwise proceed to Step 3.

### Step 3: Handle remaining issues

The **Post-fix Verification** section lists all items that need LLM intervention. For each:

- **INDEX row in wrong status group** (`状态(group)` drift): Read `docs/plan/INDEX.md`. Move the row (and its `↳` sub-plan rows) from the current group to the correct group matching the Header `状态`. Use Edit tool.
- **Orphan: document not in INDEX** (`missing_from_index`): Read the document's Header. Determine the correct INDEX group and position. Use Edit tool to add a new row.
- **Orphan: dangling INDEX entry** (`dangling_index_entries`): Report to the user. Do NOT delete the entry — let the user decide.

If there are many remaining items, run `--check --json` for machine-readable output:
```bash
python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --check --json
```

## Auto-fix vs LLM Decision Matrix

| Issue | Script auto-fixes? | LLM action |
|-------|:---:|---------------|
| Missing `执行模式` on non-checklist plan | Yes | — |
| Legacy status (实施中→active) | Yes | — |
| Wrong field order | Yes | — |
| Missing `更新日期` | Yes | — |
| INDEX version/date column mismatch | Yes | — |
| Missing `状态` | **No** | Read INDEX context, set via Edit |
| Missing `版本` | **No** | Read INDEX context, set via Edit |
| No header at all | **No** | Construct full header via Edit |
| Orphan files not in INDEX | **No** | Determine placement, add row via Edit |
| INDEX row in wrong status group | **No** | Move row(s) via Edit |
| Dangling INDEX entry | **No** | Report to user, do not delete |

## Non-Standard Entry Handling

The following entries are considered **non-standard** and must NOT be auto-rewritten:

- `README.md` files
- INDEX rows pointing to non-existent files
- INDEX rows with no link (e.g., placeholder text like `工作进展（已移除）`)
- `↳` sub-plan rows when the parent is not a standard plan
- Entries in `docs/spec/INDEX.md` that link outside `docs/spec/` (e.g., `../reports/`)

For all non-standard entries: **output a warning**, do not modify.

## Prohibited Actions

- Overwriting a valid Header field with a less precise value
- Using INDEX as a status source to set Header (INDEX is always derived, never authoritative)
- Auto-creating new INDEX groups or domain sections
- Modifying `docs/work-journal/` files
- Deleting any INDEX entries (even orphans — only report them)
