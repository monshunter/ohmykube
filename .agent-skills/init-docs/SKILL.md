---
name: init-docs
description: Initialize docs directory structure with templates for work journals, plans, specs, and other documentation. Run this skill once when setting up a new project or adding documentation infrastructure. Triggers on /init-docs.
---

# Initialize Docs Skill

Sets up the `docs/` directory structure with README.md and INDEX.md files for all documentation types.

## When to Use

- Setting up a new project
- Adding documentation infrastructure to existing project
- Before using `/work-journal`, `/create-doc`, or `/tdd` skills

## Directory Structure Created

```
docs/
├── README.md              # Main navigation hub
├── work-journal/
│   ├── README.md          # Work journal specification
│   └── INDEX.md           # Journal index (empty)
├── plan/
│   ├── README.md          # Plan specification
│   └── INDEX.md           # Plan index (empty)
├── spec/
│   ├── README.md          # Design doc specification
│   └── INDEX.md           # Spec index (empty)
├── reports/
│   ├── README.md          # Report specification
│   └── INDEX.md           # Report index (empty)
├── discuss/
│   ├── README.md          # Discussion specification
│   └── INDEX.md           # Discussion index (empty)
└── bugs/
    ├── README.md          # Bug record specification
    ├── INDEX.md           # Bug index (by module)
    └── PATTERNS.md        # Bug pattern library
```

## Workflow

### Step 1: Check existing structure

Check if `docs/` directory exists and what subdirectories are already present.

### Step 2: Create missing directories

Create any missing subdirectories from the list above.

### Step 3: Create template files

For each subdirectory, create README.md and INDEX.md from templates:

| Directory | README Template | INDEX Template |
|-----------|-----------------|----------------|
| `docs/` | [docs-readme.md](./templates/docs-readme.md) | N/A |
| `work-journal/` | [work-journal-readme.md](./templates/work-journal-readme.md) | [work-journal-index.md](./templates/work-journal-index.md) |
| `plan/` | [plan-readme.md](./templates/plan-readme.md) | [plan-index.md](./templates/plan-index.md) |
| `spec/` | [spec-readme.md](./templates/spec-readme.md) | [spec-index.md](./templates/spec-index.md) |
| `reports/` | [reports-readme.md](./templates/reports-readme.md) | [reports-index.md](./templates/reports-index.md) |
| `discuss/` | [discuss-readme.md](./templates/discuss-readme.md) | [discuss-index.md](./templates/discuss-index.md) |
| `bugs/` | [bugs-readme.md](./templates/bugs-readme.md) | [bugs-index.md](./templates/bugs-index.md) + [bugs-patterns.md](./templates/bugs-patterns.md) |

### Step 4: Report results

Report which files were created and which already existed.

## Options

User can specify which directories to initialize:

- `all` (default) - Initialize all directories
- `work-journal` - Only work-journal directory
- `plan` - Only plan directory
- `spec` - Only spec directory
- `minimal` - Only work-journal and plan (most commonly used)

## Skip Existing Files

**Never overwrite existing files**. If README.md or INDEX.md already exists, skip it and report to user.

## Post-Initialization

After running `/init-docs`, you can use:
- `/work-journal` - Record daily work progress
- `/create-doc` - Create new documents in any directory
- `/tdd` - Follow TDD workflow with plan checklists
- `/bug-report` - Create Bug knowledge base records
