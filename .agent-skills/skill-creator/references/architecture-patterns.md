# Architecture Patterns for Ferry Skills

Project-specific pattern catalog, skill mappings, and decision guidance for the Ferry skill ecosystem.

## Table of Contents

1. [Pattern Catalog](#1-pattern-catalog)
2. [Project Skill Mapping](#2-project-skill-mapping)
3. [Language Selection](#3-language-selection)
4. [Tradeoff Matrix](#4-tradeoff-matrix)
5. [Boundary Cases](#5-boundary-cases)

## 1 Pattern Catalog

### 1.1 Orchestrator (Pure SKILL.md)

**Structure**: SKILL.md only
**Core value**: Compose existing project commands, scripts, and CLI tools into a reusable workflow. No new execution logic is needed—the skill arranges what already exists.

**Characteristics**:
- All commands/tools already exist in the project
- The skill provides sequencing, prerequisite checks, and parameter guidance
- Adding scripts would duplicate existing functionality

**Project examples**: `redeploy`, `test-env`, `test-scenario`, `agent-browser`

### 1.2 Judgment (Pure SKILL.md)

**Structure**: SKILL.md only
**Core value**: LLM reasoning is the primary execution engine. The skill provides decision frameworks, templates, and constraints but delegates execution to Claude's judgment.

**Characteristics**:
- Output depends on context analysis and heuristic reasoning
- No deterministic algorithm can replace the decision process
- The skill guides *what to think about*, not *what to execute*

**Project examples**: `tdd`, `create-doc`, `bug-report`, `work-journal`, `test-investigate`

### 1.3 Deterministic (SKILL.md + scripts/)

**Structure**: SKILL.md + scripts/
**Core value**: Push verifiable, repeatable logic into scripts. LLM handles the non-deterministic parts (when to run, how to interpret results, edge case judgment).

**Characteristics**:
- Logic involves structured data parsing, transformation, or batch operations
- Correctness is auto-verifiable (diff, exit codes, schema validation)
- Same input must always produce same output

**Project example**: `sync-doc-index` (Python script parses Headers, validates INDEX projections, outputs diffs)

### 1.4 Knowledge-Enhanced (SKILL.md + references/)

**Structure**: SKILL.md + references/
**Core value**: Provide domain knowledge, normative truth sources, or evolving project context that exceeds model training data. Loaded on-demand to manage token cost.

**Characteristics**:
- Information is project-specific and not in Claude's training data
- Knowledge evolves with the project and needs periodic updates
- Progressive disclosure: SKILL.md contains the workflow; references contain depth

**Project example**: `skill-creator` uses `references/workflows.md`, `references/output-patterns.md`, and this file for project-specific architecture guidance.

### 1.5 Asset-Template (SKILL.md + assets/)

**Structure**: SKILL.md + assets/ (may combine with scripts/ or references/)
**Core value**: Provide reusable templates, scaffolding, or static resource files that are copied or adapted into output. Assets are not loaded into context—they are used directly.

**Characteristics**:
- Output includes files derived from templates or boilerplate
- Assets are copied/modified, not read for reasoning
- May combine with scripts (for generation logic) or references (for customization guidance)

**Project example**: `init-docs` uses a `templates/` directory (functionally equivalent to `assets/`) containing 16 scaffold files for all documentation types.

## 2 Project Skill Mapping

| Skill | Pattern | Structure | Rationale |
|-------|---------|-----------|-----------|
| `redeploy` | Orchestrator | SKILL.md | Arranges `make` targets + `kubectl` commands |
| `test-env` | Orchestrator | SKILL.md | Orchestrates environment lifecycle (create/verify/cleanup) |
| `test-scenario` | Orchestrator | SKILL.md | Orchestrates test execution with mode selection |
| `agent-browser` | Orchestrator | SKILL.md | Orchestrates browser automation tools |
| `tdd` | Judgment | SKILL.md | LLM drives Red-Green-Refactor cycle decisions |
| `create-doc` | Judgment | SKILL.md | LLM decides format, structure, index updates |
| `bug-report` | Judgment | SKILL.md | LLM evaluates whether bug record is needed and structures it |
| `work-journal` | Judgment | SKILL.md | LLM summarizes work and formats journal entries |
| `test-investigate` | Judgment | SKILL.md | LLM performs priority-ordered failure diagnosis |
| `init-docs` | Judgment + templates | SKILL.md + templates/ | LLM-driven with scaffold templates (see [Boundary Cases](#init-docs)) |
| `sync-doc-index` | Deterministic | SKILL.md + scripts/ | Python parses Headers and validates INDEX consistency |
| `skill-creator` | Knowledge-Enhanced | SKILL.md + scripts/ + references/ | Scripts provide scaffolding; references provide progressive knowledge |

## 3 Language Selection

When the Architecture Decision Tree leads to `scripts/`, choose between Python and Shell:

| Criterion | Choose Python | Choose Shell |
|-----------|--------------|--------------|
| Primary task shape | Structured parsing, transformation, complex branching, state machines | Command orchestration, parameter passthrough, lightweight file ops |
| Error handling needs | Fine-grained error types, recoverable flows, retry logic | Linear fail-stop (`set -e`) is sufficient |
| Testability | Unit tests expected; logic benefits from `pytest`/`unittest` | Manual verification or simple smoke tests suffice |
| Maintenance outlook | Ongoing iteration expected; multiple contributors | One-off script or stable low-complexity utility |
| Dependencies | Needs libraries (YAML, JSON parsing, HTTP) | Standard coreutils and project CLI tools suffice |

**Important**: Line count is an auxiliary signal, not a threshold. A 20-line Python script that parses YAML is correct; a 200-line Shell script doing the same is a smell. But a 50-line Shell script that chains `make`, `docker`, and `kubectl` is perfectly appropriate regardless of line count.

### Project precedent

- `sync-doc-index` chose **Python**: parses markdown Headers with regex, validates structured projections, manages state across files, has unit-testable logic.
- `skill-creator` chose **Python** for `init_skill.py` / `package_skill.py`: file generation, validation, ZIP packaging—all benefit from Python's stdlib.

## 4 Tradeoff Matrix

| Dimension | Orchestrator | Judgment | Deterministic | Knowledge-Enhanced | Asset-Template |
|-----------|:-----------:|:-------:|:------------:|:-----------------:|:-------------:|
| Maintenance cost | Lowest | Low | Medium | Medium | Low |
| Token cost (per invocation) | Low | Low | Low (scripts not loaded) | Medium (on-demand load) | Low (assets not loaded) |
| Determinism | Depends on underlying tools | Lowest | Highest | Medium | Medium |
| Adaptability | High | Highest | Lower (script changes needed) | High | Medium |
| Setup effort | Minimal | Minimal | Medium (write + test scripts) | Medium (write references) | Low (provide templates) |

**Anti-pattern: Over-engineering**. If an orchestrator skill can achieve the goal by composing existing commands, do not add scripts that wrap those same commands. The decision tree's Q1 catches this: if the answer is YES, stop at pure SKILL.md.

## 5 Boundary Cases

### `test-env`

**Classification**: Orchestrator (not Deterministic)

Despite managing complex environment lifecycle (create/verify/cleanup across L1-L5 layers), `test-env` orchestrates existing shell scripts and kubectl commands. It does not need custom scripts because:
- The underlying `image-cache.sh`, `kustomize`, and `kubectl` commands already exist
- The skill adds sequencing and prerequisite validation, not new logic
- Adding wrapper scripts would duplicate the Makefile and shell scripts

**Lesson**: Complex workflows do not automatically require `scripts/`. If the complexity lies in *when* and *in what order* to run things (not *how* to execute them), orchestrator pattern suffices.

### `agent-browser`

**Classification**: Orchestrator (not Deterministic)

Browser automation uses existing tools (Puppeteer/Playwright). The skill provides guidance on *when* to use which tool and *how* to structure interactions, but the actual browser commands are tool-native. No project scripts are needed.

### `sync-doc-index`

**Classification**: Deterministic (not Judgment)

The Header/INDEX validation logic is auto-verifiable: given input files, the expected output is deterministic. This disqualifies it from judgment pattern despite requiring some heuristic understanding of document structure. The Python script `sync-doc-index.py` encapsulates:
- Header field parsing (regex-based)
- INDEX projection validation
- Diff generation for fixes

**Lesson**: If you can write a test that says "given this input, expect this output" without human judgment, the logic belongs in a script.

### `init-docs`

**Classification**: Judgment + templates (boundary)

`init-docs` uses a `templates/` directory (16 scaffold files), which is functionally equivalent to `assets/`. It could be classified as Asset-Template pattern. However, the spec classifies it as Judgment because:
- The template files are simple scaffolds, not complex boilerplate
- The LLM judgment about *what* to initialize and *how* to adapt templates is the primary value
- The templates serve as starting points, not deterministic output

**Lesson**: When templates are simple scaffolds and the LLM's judgment dominates the workflow, the skill leans toward Judgment even if it carries template files. The `assets/` pattern is more appropriate when templates are complex, framework-specific boilerplate (e.g., React project scaffold).

### Combined architectures

Some skills may need multiple resource types. Valid combinations:
- `SKILL.md + scripts/ + references/`: Scripts for deterministic execution, references for domain knowledge (e.g., `skill-creator`)
- `SKILL.md + assets/ + references/`: Templates for output, references for customization guidance
- `SKILL.md + assets/ + scripts/`: Templates for output, scripts for generation/transformation logic

When combining, each resource type must justify its presence independently. If a reference file could be inlined into SKILL.md without exceeding the 500-line guideline, prefer inlining.
