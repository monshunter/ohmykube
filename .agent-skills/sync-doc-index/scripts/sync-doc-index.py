#!/usr/bin/env python3
"""sync-doc-index.py — Deterministic checker for document Header / INDEX drift.

Scans docs/spec/ and docs/plan/ for:
  - Header field violations (missing, wrong order, invalid values)
  - INDEX drift (Header vs INDEX mismatch)
  - Orphans (files not in INDEX, INDEX entries without files)

Usage:
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py               # default: --check
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --check       # human-readable audit
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --check --json  # JSON audit
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-header    # auto-fix headers
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-header --dry-run
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-index     # auto-fix INDEX columns
  python3 .agent-skills/sync-doc-index/scripts/sync-doc-index.py --fix-index --dry-run
"""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

# ── Constants ──────────────────────────────────────────────────────────

VALID_STATUS = {"draft", "active", "completed", "superseded", "deprecated"}
LEGACY_STATUS = {"实施中": "active", "已完成": "completed", "废弃": "deprecated"}
EXEC_MODES = {"parallel", "sequential"}
DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
HEADER_RE = re.compile(r"^>\s*\*\*(.+?)\*\*\s*:\s*(.+)$")
LINK_RE = re.compile(r"\[([^\]]*)\]\(([^)]+)\)")

SPEC_FIELDS = ["版本", "状态", "更新日期"]
PLAN_FIELDS = ["版本", "状态", "更新日期", "执行模式"]
CHECKLIST_FIELDS = ["版本", "状态", "更新日期"]
SKIP_FILES = {"README.md", "INDEX.md"}

PLAN_GROUP_STATUS = {
    "进行中": "active",
    "Active": "active",
    "草稿": "draft",
    "Draft": "draft",
    "已完成": "completed",
    "Completed": "completed",
    "已取代": "superseded",
    "Superseded": "superseded",
}


# ── Header Parser ─────────────────────────────────────────────────────


def parse_header(filepath):
    """Extract header fields from a markdown file.

    Returns dict with 'fields' (ordered dict-like list of tuples),
    'standard' (dict of recognized fields), 'order' (list of field names),
    and 'adjacent' (bool: whether header immediately follows the title line).
    """
    fields = []
    in_header = False
    gap_seen = False
    title_seen = False
    lines_after_title = 0  # non-empty lines between title and first header field

    with open(filepath, "r", encoding="utf-8") as f:
        for line in f:
            stripped = line.strip()
            if stripped.startswith("# ") and not in_header and not fields:
                title_seen = True
                continue
            if title_seen and not in_header and not fields:
                # Between title and first header field
                if stripped and not HEADER_RE.match(stripped):
                    lines_after_title += 1
            m = HEADER_RE.match(stripped)
            if m:
                in_header = True
                gap_seen = False
                fields.append((m.group(1).strip(), m.group(2).strip()))
            elif in_header and stripped == "":
                gap_seen = True
            elif in_header and gap_seen and not stripped.startswith(">"):
                break
            elif in_header and not stripped.startswith(">") and stripped != "":
                break

    order = [name for name, _ in fields]
    standard = {name: val for name, val in fields if name in ("版本", "状态", "更新日期", "执行模式")}
    adjacent = lines_after_title == 0
    return {"fields": fields, "standard": standard, "order": order, "adjacent": adjacent}


# ── INDEX Parsers ──────────────────────────────────────────────────────


SEPARATOR_RE = re.compile(r"^\|[\s\-:|]+\|$")


def parse_spec_index(index_path):
    """Parse spec INDEX.md. Returns list of {name, file, version, status, date, non_standard}."""
    entries = []
    with open(index_path, "r", encoding="utf-8") as f:
        all_lines = f.readlines()

    # Build set of table-header line indices (line immediately before a separator row)
    header_lines = set()
    for i, line in enumerate(all_lines):
        if SEPARATOR_RE.match(line.strip()):
            if i > 0:
                header_lines.add(i - 1)

    for i, line in enumerate(all_lines):
        stripped = line.strip()
        if not stripped.startswith("|"):
            continue
        # Skip separator rows and table header rows
        if SEPARATOR_RE.match(stripped) or i in header_lines:
            continue
        cells = [c.strip() for c in stripped.split("|")]
        cells = [c for c in cells if c]  # remove empty from leading/trailing |
        if len(cells) < 4:
            continue

        doc_cell, version, status, date = cells[0], cells[1], cells[2], cells[3]
        links = LINK_RE.findall(doc_cell)
        file_path = links[0][1] if links else None
        non_standard = (
            file_path is None
            or file_path.startswith("../")
            or file_path == ""
        )
        entries.append({
            "name": links[0][0] if links else doc_cell,
            "file": file_path,
            "version": version,
            "status": status,
            "date": date,
            "non_standard": non_standard,
        })
    return entries


def parse_plan_index(index_path):
    """Parse plan INDEX.md. Returns list of entries with status group info."""
    entries = []
    current_status = None
    has_version_col = True  # Superseded group has no version/date columns

    with open(index_path, "r", encoding="utf-8") as f:
        all_lines = f.readlines()

    # Build set of table-header line indices (line immediately before a separator row)
    header_lines = set()
    for i, line in enumerate(all_lines):
        if SEPARATOR_RE.match(line.strip()):
            if i > 0:
                header_lines.add(i - 1)

    for i, line in enumerate(all_lines):
        stripped = line.strip()

        # Detect group headings: ## 1 进行中（Active）
        section_match = re.match(r"^##\s+\d+\s+(.+)$", stripped)
        if section_match:
            section_name = section_match.group(1)
            current_status = None
            has_version_col = True
            for keyword, status in PLAN_GROUP_STATUS.items():
                if keyword in section_name:
                    current_status = status
                    if status == "superseded":
                        has_version_col = False
                    break
            continue

        if current_status is None or not stripped.startswith("|"):
            continue
        # Skip separator rows and table header rows
        if SEPARATOR_RE.match(stripped) or i in header_lines:
            continue

        cells = [c.strip() for c in stripped.split("|")]
        cells = [c for c in cells if c]
        if not cells:
            continue

        plan_cell = cells[0]
        files_cell = cells[1] if len(cells) > 1 else ""
        version = cells[2] if len(cells) > 2 and has_version_col else ""
        date = cells[3] if len(cells) > 3 and has_version_col else ""
        is_sub = plan_cell.strip().startswith("↳")

        # Extract all linked files
        file_links = LINK_RE.findall(files_cell)
        linked_files = [url for _, url in file_links]

        entries.append({
            "name": plan_cell,
            "is_sub": is_sub,
            "expected_status": current_status,
            "linked_files": linked_files,
            "version": version,
            "date": date,
            "has_version_col": has_version_col,
            "non_standard": not linked_files,
        })
    return entries


# ── Validators ─────────────────────────────────────────────────────────


def is_checklist(filepath):
    name = filepath.name
    return name.endswith("-checklist.md") or name == "checklist.md"


def validate_header(filepath, header, required_fields, root):
    """Validate header fields. Returns list of violation dicts."""
    violations = []
    rel = str(filepath.relative_to(root))
    std = header["standard"]
    order = header["order"]

    # Missing fields
    for f in required_fields:
        if f not in std:
            auto = f in ("执行模式", "更新日期")
            violations.append({
                "file": rel,
                "type": "missing_field",
                "field": f,
                "auto_fixable": auto,
                "message": f"Missing required field: {f}",
            })

    # Field order
    present_standard = [f for f in order if f in set(required_fields)]
    expected_order = [f for f in required_fields if f in present_standard]
    if present_standard != expected_order:
        violations.append({
            "file": rel,
            "type": "wrong_order",
            "auto_fixable": True,
            "expected": required_fields,
            "actual": present_standard,
            "message": f"Wrong field order: {present_standard} (expected {expected_order})",
        })

    # Status validation
    if "状态" in std:
        s = std["状态"]
        if s in LEGACY_STATUS:
            violations.append({
                "file": rel,
                "type": "legacy_status",
                "field": "状态",
                "current": s,
                "normalized": LEGACY_STATUS[s],
                "auto_fixable": True,
                "message": f"Legacy status '{s}' → '{LEGACY_STATUS[s]}'",
            })
        elif s not in VALID_STATUS:
            violations.append({
                "file": rel,
                "type": "invalid_status",
                "value": s,
                "auto_fixable": False,
                "message": f"Invalid status: {s}",
            })

    # Date validation
    if "更新日期" in std and not DATE_RE.match(std["更新日期"]):
        violations.append({
            "file": rel,
            "type": "invalid_date",
            "value": std["更新日期"],
            "auto_fixable": False,
            "message": f"Invalid date format: {std['更新日期']}",
        })

    # 执行模式 validation
    if "执行模式" in std and std["执行模式"] not in EXEC_MODES:
        violations.append({
            "file": rel,
            "type": "invalid_exec_mode",
            "value": std["执行模式"],
            "auto_fixable": False,
            "message": f"Invalid 执行模式: {std['执行模式']}",
        })

    # No header at all
    if not header["fields"]:
        violations.append({
            "file": rel,
            "type": "no_header",
            "auto_fixable": False,
            "message": "No header block found",
        })

    # Header not adjacent to title (may have captured fields from body)
    if header["fields"] and not header.get("adjacent", True):
        violations.append({
            "file": rel,
            "type": "header_not_adjacent",
            "auto_fixable": False,
            "message": "Header block is not immediately after the title line (may be unreliable)",
        })

    return violations


# ── Drift Checker ──────────────────────────────────────────────────────


def check_spec_drift(spec_dir, root):
    """Check spec INDEX vs document headers. Returns (drifts, warnings)."""
    index_path = spec_dir / "INDEX.md"
    if not index_path.exists():
        return [], [{"entry": str(index_path), "reason": "INDEX.md not found"}]

    entries = parse_spec_index(index_path)
    drifts = []
    warnings = []

    for entry in entries:
        if entry["non_standard"]:
            warnings.append({"entry": entry["name"], "reason": "non-standard entry (external link or no link)"})
            continue

        doc_path = (spec_dir / entry["file"]).resolve()
        if not doc_path.exists():
            warnings.append({"entry": entry["name"], "file": entry["file"], "reason": "linked file does not exist"})
            continue

        header = parse_header(doc_path)
        std = header["standard"]
        rel = str(doc_path.relative_to(root))

        for field, idx_key in [("版本", "version"), ("状态", "status"), ("更新日期", "date")]:
            if field in std and std[field] != entry[idx_key]:
                drifts.append({
                    "file": rel,
                    "field": field,
                    "header_value": std[field],
                    "index_value": entry[idx_key],
                    "auto_fixable": True,
                })

    return drifts, warnings


def check_plan_drift(plan_dir, root):
    """Check plan INDEX vs document headers. Returns (drifts, warnings)."""
    index_path = plan_dir / "INDEX.md"
    if not index_path.exists():
        return [], [{"entry": str(index_path), "reason": "INDEX.md not found"}]

    entries = parse_plan_index(index_path)
    drifts = []
    warnings = []

    for entry in entries:
        if entry["non_standard"]:
            warnings.append({"entry": entry["name"], "reason": "non-standard entry"})
            continue

        # Check all linked files for status group drift
        for linked_file in entry["linked_files"]:
            doc_path = (plan_dir / linked_file).resolve()
            if not doc_path.exists():
                continue

            header = parse_header(doc_path)
            std = header["standard"]
            rel = str(doc_path.relative_to(root))

            # Status group drift
            if "状态" in std and std["状态"] != entry["expected_status"]:
                severity = "advisory" if entry["is_sub"] else "error"
                drifts.append({
                    "file": rel,
                    "field": "状态(group)",
                    "header_value": std["状态"],
                    "index_value": entry["expected_status"],
                    "auto_fixable": False,
                    "severity": severity,
                })

        # Version/date drift: use first existing linked file
        for linked_file in entry["linked_files"]:
            doc_path = (plan_dir / linked_file).resolve()
            if not doc_path.exists():
                continue

            header = parse_header(doc_path)
            std = header["standard"]
            rel = str(doc_path.relative_to(root))

            # Version drift (skip for groups without version column, e.g. Superseded)
            if entry.get("has_version_col", True) and "版本" in std and entry["version"] and std["版本"] != entry["version"]:
                drifts.append({
                    "file": rel,
                    "field": "版本",
                    "header_value": std["版本"],
                    "index_value": entry["version"],
                    "auto_fixable": True,
                })

            # Date drift (skip for groups without date column)
            if entry.get("has_version_col", True) and "更新日期" in std and entry["date"] and std["更新日期"] != entry["date"]:
                drifts.append({
                    "file": rel,
                    "field": "更新日期",
                    "header_value": std["更新日期"],
                    "index_value": entry["date"],
                    "auto_fixable": True,
                })

            break  # Version/date columns represent the first file

    return drifts, warnings


# ── Orphan Detector ────────────────────────────────────────────────────


def detect_orphans(spec_dir, plan_dir, root):
    """Detect files not in INDEX and INDEX entries pointing to missing files."""
    missing_from_index = []
    dangling = []

    # Spec orphans
    spec_index_path = spec_dir / "INDEX.md"
    if spec_index_path.exists():
        entries = parse_spec_index(spec_index_path)
        indexed_files = {e["file"] for e in entries if e["file"]}

        for md in sorted(spec_dir.glob("*.md")):
            if md.name in SKIP_FILES:
                continue
            rel_to_spec = "./" + md.name
            if rel_to_spec not in indexed_files:
                missing_from_index.append(str(md.relative_to(root)))

        for e in entries:
            if e["file"] and not e["non_standard"]:
                full = (spec_dir / e["file"]).resolve()
                if not full.exists():
                    dangling.append(e["file"])

    # Plan orphans
    plan_index_path = plan_dir / "INDEX.md"
    if plan_index_path.exists():
        entries = parse_plan_index(plan_index_path)
        indexed_files = set()
        for e in entries:
            for lf in e["linked_files"]:
                indexed_files.add(lf)

        for md in sorted(plan_dir.rglob("*.md")):
            if md.name in SKIP_FILES:
                continue
            # Skip files not directly under plan subdirs
            rel_to_plan = md.relative_to(plan_dir)
            if len(rel_to_plan.parts) < 2:
                continue
            rel_link = "./" + str(rel_to_plan)
            if rel_link not in indexed_files:
                missing_from_index.append(str(md.relative_to(root)))

        for e in entries:
            for lf in e["linked_files"]:
                full = (plan_dir / lf).resolve()
                if not full.exists():
                    dangling.append(lf)

    return {"missing_from_index": missing_from_index, "dangling_index_entries": dangling}


# ── Full Check ─────────────────────────────────────────────────────────


def run_check(root):
    """Run full check. Returns report dict."""
    spec_dir = root / "docs" / "spec"
    plan_dir = root / "docs" / "plan"

    violations = []
    all_warnings = []

    # Spec headers
    for md in sorted(spec_dir.glob("*.md")):
        if md.name in SKIP_FILES:
            continue
        header = parse_header(md)
        violations.extend(validate_header(md, header, SPEC_FIELDS, root))

    # Plan headers
    for md in sorted(plan_dir.rglob("*.md")):
        if md.name in SKIP_FILES:
            continue
        rel = md.relative_to(plan_dir)
        if len(rel.parts) < 2:
            continue
        required = CHECKLIST_FIELDS if is_checklist(md) else PLAN_FIELDS
        header = parse_header(md)
        violations.extend(validate_header(md, header, required, root))

    # INDEX drifts
    spec_drifts, spec_warns = check_spec_drift(spec_dir, root)
    plan_drifts, plan_warns = check_plan_drift(plan_dir, root)
    drifts = spec_drifts + plan_drifts
    all_warnings.extend(spec_warns)
    all_warnings.extend(plan_warns)

    # Orphans
    orphans = detect_orphans(spec_dir, plan_dir, root)

    auto_fixable = sum(1 for v in violations if v.get("auto_fixable")) + sum(1 for d in drifts if d.get("auto_fixable"))
    needs_llm = (
        sum(1 for v in violations if not v.get("auto_fixable"))
        + sum(1 for d in drifts if not d.get("auto_fixable"))
        + len(orphans["missing_from_index"])
        + len(orphans["dangling_index_entries"])
    )

    return {
        "header_violations": violations,
        "index_drifts": drifts,
        "orphans": orphans,
        "warnings": all_warnings,
        "summary": {
            "violations": len(violations),
            "drifts": len(drifts),
            "orphans": len(orphans["missing_from_index"]) + len(orphans["dangling_index_entries"]),
            "warnings": len(all_warnings),
            "auto_fixable": auto_fixable,
            "needs_llm": needs_llm,
        },
    }


# ── Auto-Fixers ────────────────────────────────────────────────────────


def git_last_modified(filepath):
    """Get last modified date from git log."""
    try:
        result = subprocess.run(
            ["git", "log", "--follow", "-1", "--format=%cs", str(filepath)],
            capture_output=True, text=True, timeout=10,
        )
        date = result.stdout.strip()
        if DATE_RE.match(date):
            return date
    except Exception:
        pass
    return None


def fix_header_file(filepath, header, required_fields, dry_run=False):
    """Fix header of a single file. Returns (fixes, skipped) tuple."""
    fixes = []
    skipped = []
    std = header["standard"]

    with open(filepath, "r", encoding="utf-8") as f:
        lines = f.readlines()

    modified = False

    # 1. Add missing 执行模式 (only for non-checklist plan docs)
    if "执行模式" in required_fields and "执行模式" not in std:
        found = False
        for i, line in enumerate(lines):
            m = HEADER_RE.match(line.strip())
            if m and m.group(1).strip() == "更新日期":
                insert_line = "> **执行模式**: sequential\n"
                lines.insert(i + 1, insert_line)
                fixes.append({"action": "add_field", "field": "执行模式", "value": "sequential"})
                modified = True
                found = True
                break
        if not found:
            skipped.append({"action": "add_field", "field": "执行模式", "reason": "no 更新日期 line found as anchor"})

    # 2. Normalize legacy status
    for i, line in enumerate(lines):
        m = HEADER_RE.match(line.strip())
        if m and m.group(1).strip() == "状态":
            val = m.group(2).strip()
            if val in LEGACY_STATUS:
                new_val = LEGACY_STATUS[val]
                lines[i] = line.replace(f"**: {val}", f"**: {new_val}")
                fixes.append({"action": "normalize_status", "from": val, "to": new_val})
                modified = True

    # 3. Recover missing 更新日期 from git
    if "更新日期" in required_fields and "更新日期" not in std:
        date = git_last_modified(filepath)
        if date:
            insert_after = None
            for i, line in enumerate(lines):
                m = HEADER_RE.match(line.strip())
                if m and m.group(1).strip() in ("状态", "版本"):
                    insert_after = i
            if insert_after is not None:
                lines.insert(insert_after + 1, f"> **更新日期**: {date}\n")
                fixes.append({"action": "add_field", "field": "更新日期", "value": date, "source": "git_log"})
                modified = True
            else:
                skipped.append({"action": "add_field", "field": "更新日期", "reason": "no 状态/版本 line found as anchor"})
        else:
            skipped.append({"action": "add_field", "field": "更新日期", "reason": "git log returned no date"})

    # 4. Fix field order — always re-parse from current lines for consistency
    # Build a temporary file-like header from current lines (not the stale input header)
    current_fields = []
    for line in lines:
        m = HEADER_RE.match(line.strip())
        if m:
            current_fields.append((m.group(1).strip(), m.group(2).strip()))
    current_order = [name for name, _ in current_fields]
    present_standard = [f for f in current_order if f in set(required_fields)]
    expected_order = [f for f in required_fields if f in present_standard]

    if present_standard != expected_order:
        header_lines_list = []
        header_start = None
        header_end = None
        for i, line in enumerate(lines):
            m = HEADER_RE.match(line.strip())
            if m:
                if header_start is None:
                    header_start = i
                header_end = i
                header_lines_list.append((m.group(1).strip(), line))

        if header_start is not None:
            standard_lines = {name: line for name, line in header_lines_list if name in set(required_fields)}
            non_standard_lines = [(name, line) for name, line in header_lines_list if name not in set(required_fields)]

            reordered = []
            for f in required_fields:
                if f in standard_lines:
                    reordered.append(standard_lines[f])
            for _, line in non_standard_lines:
                reordered.append(line)

            for idx, i in enumerate(range(header_start, header_end + 1)):
                if idx < len(reordered):
                    lines[i] = reordered[idx]

            fixes.append({"action": "reorder_fields"})
            modified = True

    if modified and not dry_run:
        with open(filepath, "w", encoding="utf-8") as f:
            f.writelines(lines)

    return fixes, skipped


def fix_headers(root, dry_run=False):
    """Auto-fix all fixable header violations. Returns (all_fixes, all_skipped)."""
    spec_dir = root / "docs" / "spec"
    plan_dir = root / "docs" / "plan"
    all_fixes = []
    all_skipped = []

    # Spec docs
    for md in sorted(spec_dir.glob("*.md")):
        if md.name in SKIP_FILES:
            continue
        header = parse_header(md)
        violations = validate_header(md, header, SPEC_FIELDS, root)
        fixable = [v for v in violations if v.get("auto_fixable")]
        if fixable:
            fixes, skipped = fix_header_file(md, header, SPEC_FIELDS, dry_run)
            rel = str(md.relative_to(root))
            if fixes:
                all_fixes.append({"file": rel, "fixes": fixes})
            if skipped:
                all_skipped.append({"file": rel, "skipped": skipped})

    # Plan docs
    for md in sorted(plan_dir.rglob("*.md")):
        if md.name in SKIP_FILES:
            continue
        rel = md.relative_to(plan_dir)
        if len(rel.parts) < 2:
            continue
        required = CHECKLIST_FIELDS if is_checklist(md) else PLAN_FIELDS
        header = parse_header(md)
        violations = validate_header(md, header, required, root)
        fixable = [v for v in violations if v.get("auto_fixable")]
        if fixable:
            fixes, skipped = fix_header_file(md, header, required, dry_run)
            rel_str = str(md.relative_to(root))
            if fixes:
                all_fixes.append({"file": rel_str, "fixes": fixes})
            if skipped:
                all_skipped.append({"file": rel_str, "skipped": skipped})

    return all_fixes, all_skipped


def fix_index_columns(root, dry_run=False):
    """Auto-fix INDEX column values (version/date) to match headers. Returns summary."""
    fixes = []

    # Spec INDEX
    spec_dir = root / "docs" / "spec"
    spec_index = spec_dir / "INDEX.md"
    if spec_index.exists():
        spec_fixes = _fix_spec_index(spec_dir, spec_index, root, dry_run)
        fixes.extend(spec_fixes)

    # Plan INDEX
    plan_dir = root / "docs" / "plan"
    plan_index = plan_dir / "INDEX.md"
    if plan_index.exists():
        plan_fixes = _fix_plan_index(plan_dir, plan_index, root, dry_run)
        fixes.extend(plan_fixes)

    return fixes


def _fix_spec_index(spec_dir, index_path, root, dry_run):
    """Fix spec INDEX column values."""
    fixes = []
    with open(index_path, "r", encoding="utf-8") as f:
        lines = f.readlines()

    # Build set of table-header line indices
    header_lines = set()
    for i, line in enumerate(lines):
        if SEPARATOR_RE.match(line.strip()):
            if i > 0:
                header_lines.add(i - 1)

    modified = False
    for i, line in enumerate(lines):
        stripped = line.strip()
        if not stripped.startswith("|"):
            continue
        if SEPARATOR_RE.match(stripped) or i in header_lines:
            continue
        cells = [c.strip() for c in stripped.split("|")]
        cells_raw = [c for c in cells if c]
        if len(cells_raw) < 4:
            continue

        links = LINK_RE.findall(cells_raw[0])
        if not links:
            continue
        file_path = links[0][1]
        if file_path.startswith("../"):
            continue

        doc_path = (spec_dir / file_path).resolve()
        if not doc_path.exists():
            continue

        header = parse_header(doc_path)
        std = header["standard"]
        idx_ver, idx_status, idx_date = cells_raw[1], cells_raw[2], cells_raw[3]
        new_ver = std.get("版本", idx_ver)
        new_status = std.get("状态", idx_status)
        new_date = std.get("更新日期", idx_date)

        if new_ver != idx_ver or new_status != idx_status or new_date != idx_date:
            # Rebuild the line
            new_line = f"| {cells_raw[0]} | {new_ver} | {new_status} | {new_date} |\n"
            lines[i] = new_line
            modified = True
            fixes.append({
                "index": "docs/spec/INDEX.md",
                "file": file_path,
                "changes": {
                    k: {"from": old, "to": new}
                    for k, old, new in [("版本", idx_ver, new_ver), ("状态", idx_status, new_status), ("更新日期", idx_date, new_date)]
                    if old != new
                },
            })

    if modified and not dry_run:
        with open(index_path, "w", encoding="utf-8") as f:
            f.writelines(lines)

    return fixes


def _fix_plan_index(plan_dir, index_path, root, dry_run):
    """Fix plan INDEX column values (version/date only, not group moves)."""
    fixes = []
    with open(index_path, "r", encoding="utf-8") as f:
        lines = f.readlines()

    # Build set of table-header line indices
    header_lines = set()
    for i, line in enumerate(lines):
        if SEPARATOR_RE.match(line.strip()):
            if i > 0:
                header_lines.add(i - 1)

    modified = False
    for i, line in enumerate(lines):
        stripped = line.strip()
        if not stripped.startswith("|"):
            continue
        if SEPARATOR_RE.match(stripped) or i in header_lines:
            continue
        cells = [c.strip() for c in stripped.split("|")]
        cells_raw = [c for c in cells if c]
        if len(cells_raw) < 4:
            continue

        files_cell = cells_raw[1]
        file_links = LINK_RE.findall(files_cell)
        if not file_links:
            continue

        # Check first linked file
        first_file = file_links[0][1]
        doc_path = (plan_dir / first_file).resolve()
        if not doc_path.exists():
            continue

        header = parse_header(doc_path)
        std = header["standard"]
        idx_ver, idx_date = cells_raw[2], cells_raw[3]
        new_ver = std.get("版本", idx_ver)
        new_date = std.get("更新日期", idx_date)

        if new_ver != idx_ver or new_date != idx_date:
            new_line = f"| {cells_raw[0]} | {cells_raw[1]} | {new_ver} | {new_date} |\n"
            lines[i] = new_line
            modified = True
            fixes.append({
                "index": "docs/plan/INDEX.md",
                "file": first_file,
                "changes": {
                    k: {"from": old, "to": new}
                    for k, old, new in [("版本", idx_ver, new_ver), ("更新日期", idx_date, new_date)]
                    if old != new
                },
            })

    if modified and not dry_run:
        with open(index_path, "w", encoding="utf-8") as f:
            f.writelines(lines)

    return fixes


# ── Output Formatters ──────────────────────────────────────────────────


def format_human(report):
    """Format report as human-readable text."""
    lines = []

    lines.append("## Header Violations")
    if report["header_violations"]:
        for v in report["header_violations"]:
            auto = " [auto-fixable]" if v.get("auto_fixable") else " [needs LLM]"
            lines.append(f"  - {v['file']}: {v['message']}{auto}")
    else:
        lines.append("  (none)")

    lines.append("")
    lines.append("## INDEX Drifts")
    if report["index_drifts"]:
        for d in report["index_drifts"]:
            auto = " [auto-fixable]" if d.get("auto_fixable") else " [needs LLM]"
            lines.append(f"  - {d['file']}: {d['field']} header={d['header_value']} index={d['index_value']}{auto}")
    else:
        lines.append("  (none)")

    lines.append("")
    lines.append("## Orphans")
    orphans = report["orphans"]
    if orphans["missing_from_index"] or orphans["dangling_index_entries"]:
        for f in orphans["missing_from_index"]:
            lines.append(f"  - [not in INDEX] {f}")
        for f in orphans["dangling_index_entries"]:
            lines.append(f"  - [dangling] {f}")
    else:
        lines.append("  (none)")

    lines.append("")
    lines.append("## Warnings")
    if report["warnings"]:
        for w in report["warnings"]:
            lines.append(f"  - {w.get('entry', '')}: {w['reason']}")
    else:
        lines.append("  (none)")

    lines.append("")
    s = report["summary"]
    if s["violations"] == 0 and s["drifts"] == 0 and s["orphans"] == 0:
        lines.append("All documents are in sync. Zero drift detected.")
    else:
        lines.append(f"Summary: {s['violations']} violations, {s['drifts']} drifts, {s['orphans']} orphans, {s['warnings']} warnings")
        lines.append(f"  auto-fixable: {s['auto_fixable']}, needs LLM: {s['needs_llm']}")

    return "\n".join(lines)


def format_fix_result(fixes, skipped, mode, dry_run):
    """Format fix results as structured human-readable text with 3 sections."""
    lines = []
    prefix = "[DRY RUN] " if dry_run else ""

    # Section 1: Applied fixes
    lines.append(f"## {prefix}Applied ({mode})")
    if fixes:
        lines.append(f"  {len(fixes)} file(s) {'would be ' if dry_run else ''}modified:")
        for fix in fixes:
            if "file" in fix and "fixes" in fix:
                lines.append(f"  {fix['file']}:")
                for f in fix["fixes"]:
                    lines.append(f"    - {f.get('action', 'unknown')}: {json.dumps({k: v for k, v in f.items() if k != 'action'}, ensure_ascii=False)}")
            elif "index" in fix:
                lines.append(f"  {fix['index']} → {fix.get('file', '?')}:")
                for field, change in fix.get("changes", {}).items():
                    lines.append(f"    - {field}: {change['from']} → {change['to']}")
    else:
        lines.append("  (none)")

    # Section 2: Skipped (auto-fix attempted but failed)
    lines.append("")
    lines.append(f"## {prefix}Skipped (needs LLM)")
    if skipped:
        lines.append(f"  {len(skipped)} file(s) with unresolved issues:")
        for item in skipped:
            lines.append(f"  {item['file']}:")
            for s in item["skipped"]:
                lines.append(f"    - {s.get('action', 'unknown')}: field={s.get('field', '?')} reason={s.get('reason', '?')}")
    else:
        lines.append("  (none)")

    return "\n".join(lines)


# ── Main ───────────────────────────────────────────────────────────────


def find_root():
    """Find project root by looking for docs/ directory."""
    cwd = Path.cwd()
    for candidate in [cwd, *cwd.parents]:
        if (candidate / "docs" / "spec").is_dir() and (candidate / "docs" / "plan").is_dir():
            return candidate
    print("Error: Cannot find project root (no docs/spec/ and docs/plan/ found)", file=sys.stderr)
    sys.exit(1)


def main():
    parser = argparse.ArgumentParser(description="Check and fix document Header / INDEX drift")
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--check", action="store_true", default=True, help="Audit mode (default)")
    group.add_argument("--fix-header", action="store_true", help="Auto-fix header violations")
    group.add_argument("--fix-index", action="store_true", help="Auto-fix INDEX column drifts")
    parser.add_argument("--json", action="store_true", help="JSON output (with --check)")
    parser.add_argument("--dry-run", action="store_true", help="Preview changes without writing (with --fix-*)")
    args = parser.parse_args()

    root = find_root()

    if args.fix_header:
        fixes, skipped = fix_headers(root, dry_run=args.dry_run)
        print(format_fix_result(fixes, skipped, "--fix-header", args.dry_run))
        # Section 3: Post-fix verification
        print("")
        print("## Post-fix Verification")
        report = run_check(root)
        s = report["summary"]
        if s["violations"] == 0 and s["drifts"] == 0 and s["orphans"] == 0:
            print("  All documents are in sync. Zero drift detected.")
        else:
            print(f"  {s['violations']} violations, {s['drifts']} drifts, {s['orphans']} orphans, {s['warnings']} warnings")
            print(f"  auto-fixable: {s['auto_fixable']}, needs LLM: {s['needs_llm']}")
            for v in report["header_violations"]:
                if not v.get("auto_fixable"):
                    print(f"  - [header] {v['file']}: {v['message']}")
            for d in report["index_drifts"]:
                if not d.get("auto_fixable"):
                    print(f"  - [drift] {d['file']}: {d['field']} header={d['header_value']} index={d['index_value']}")
            for f in report["orphans"]["missing_from_index"]:
                print(f"  - [orphan] {f}")
    elif args.fix_index:
        fixes = fix_index_columns(root, dry_run=args.dry_run)
        print(format_fix_result(fixes, [], "--fix-index", args.dry_run))
        # Section 3: Post-fix verification
        print("")
        print("## Post-fix Verification")
        report = run_check(root)
        s = report["summary"]
        if s["violations"] == 0 and s["drifts"] == 0 and s["orphans"] == 0:
            print("  All documents are in sync. Zero drift detected.")
        else:
            print(f"  {s['violations']} violations, {s['drifts']} drifts, {s['orphans']} orphans, {s['warnings']} warnings")
            print(f"  auto-fixable: {s['auto_fixable']}, needs LLM: {s['needs_llm']}")
            for d in report["index_drifts"]:
                if not d.get("auto_fixable"):
                    print(f"  - [drift] {d['file']}: {d['field']} header={d['header_value']} index={d['index_value']}")
            for f in report["orphans"]["missing_from_index"]:
                print(f"  - [orphan] {f}")
    else:
        report = run_check(root)
        if args.json:
            print(json.dumps(report, ensure_ascii=False, indent=2))
        else:
            print(format_human(report))


if __name__ == "__main__":
    main()
