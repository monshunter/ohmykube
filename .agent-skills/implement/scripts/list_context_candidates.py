#!/usr/bin/env python3
"""List candidate plans for /implement (no-arg mode).

Reads docs/plan/INDEX.md to find Active + Draft plans,
filters to those with context.yaml, and outputs a numbered
candidate list with reasons.

Usage:
    python3 list_context_candidates.py \
      --plan-index docs/plan/INDEX.md \
      --plan-root docs/plan
"""

import argparse
import os
import re
import sys

try:
    import yaml
except ImportError:
    print(
        "ERROR: PyYAML is not installed.\n"
        "Install it with: pip3 install PyYAML",
        file=sys.stderr,
    )
    sys.exit(2)


def parse_index(index_path: str) -> list[dict]:
    """Parse INDEX.md to extract plan entries from Active and Draft sections.

    Returns list of dicts with keys: name, dir_name, status, version, date
    """
    if not os.path.isfile(index_path):
        print(f"ERROR: INDEX file not found: {index_path}", file=sys.stderr)
        sys.exit(1)

    with open(index_path, "r", encoding="utf-8") as f:
        content = f.read()

    entries = []
    current_status = None

    for line in content.splitlines():
        stripped = line.strip()

        # Detect section headers
        if re.match(r"^##\s+\d+\s+进行中", stripped):
            current_status = "Active"
            continue
        elif re.match(r"^##\s+\d+\s+草稿", stripped):
            current_status = "Draft"
            continue
        elif re.match(r"^##\s+\d+\s+已完成", stripped):
            current_status = None  # Stop collecting
            continue
        elif re.match(r"^##\s+\d+\s+已取代", stripped):
            current_status = None
            continue

        if current_status is None:
            continue

        # Skip sub-plan rows (↳) and header rows
        if (
            stripped.startswith("| ↳")
            or stripped.startswith("|--")
            or stripped.startswith("| 计划")
        ):
            continue

        # Parse table rows: | [Name](./dir/) | ... | version | date |
        match = re.match(
            r"\|\s*\[([^\]]+)\]\(\./([^/]+)/\)\s*\|.*?\|\s*([\d.]+)\s*\|\s*(\d{4}-\d{2}-\d{2})\s*\|",
            stripped,
        )
        if match:
            name = match.group(1)
            dir_name = match.group(2)
            version = match.group(3)
            date = match.group(4)
            entries.append(
                {
                    "name": name,
                    "dir_name": dir_name,
                    "status": current_status,
                    "version": version,
                    "date": date,
                }
            )

    return entries


def count_checklist_progress(plan_dir: str) -> tuple[int, int] | None:
    """Count checked/total items across all checklist files in a plan directory.

    Returns (checked, total) or None if no checklists found.
    """
    checked = 0
    total = 0

    for fname in os.listdir(plan_dir):
        if not fname.endswith("-checklist.md"):
            continue
        fpath = os.path.join(plan_dir, fname)
        with open(fpath, "r", encoding="utf-8") as f:
            for line in f:
                stripped = line.strip()
                if stripped.startswith("- [x]") or stripped.startswith("- [X]"):
                    checked += 1
                    total += 1
                elif stripped.startswith("- [ ]"):
                    total += 1

    if total == 0:
        return None
    return checked, total


def load_target_metadata(context_path: str) -> tuple[list[str], str]:
    """Load target names and manifest status from context.yaml."""
    try:
        with open(context_path, "r", encoding="utf-8") as f:
            manifest = yaml.safe_load(f)
    except (yaml.YAMLError, OSError):
        return [], "invalid"

    if not isinstance(manifest, dict):
        return [], "invalid"

    spec = manifest.get("spec")
    if not isinstance(spec, dict):
        return [], "invalid"

    targets = spec.get("targets")
    if not isinstance(targets, dict):
        return [], "invalid"

    return sorted(targets.keys()), "ready"


def count_all_contexts(plan_root: str) -> int:
    """Count all docs/plan/*/context.yaml on disk."""
    if not os.path.isdir(plan_root):
        return 0
    total = 0
    for entry in os.listdir(plan_root):
        plan_dir = os.path.join(plan_root, entry)
        if not os.path.isdir(plan_dir):
            continue
        if os.path.isfile(os.path.join(plan_dir, "context.yaml")):
            total += 1
    return total


def main():
    parser = argparse.ArgumentParser(
        description="List candidate plans for /implement"
    )
    parser.add_argument(
        "--plan-index", default="docs/plan/INDEX.md", help="Path to docs/plan/INDEX.md"
    )
    parser.add_argument(
        "--plan-root", default="docs/plan", help="Path to docs/plan/ directory"
    )
    parser.add_argument(
        "--max-candidates",
        type=int,
        default=5,
        help="Maximum number of latest candidates to recommend (default: 5)",
    )
    args = parser.parse_args()

    if args.max_candidates <= 0:
        print("ERROR: --max-candidates must be > 0", file=sys.stderr)
        sys.exit(2)

    entries = parse_index(args.plan_index)
    if not entries:
        print(
            "No plans with context.yaml found in Active/Draft status.\n"
            "To make a plan available for /implement:\n"
            "1. Ensure the plan is registered in docs/plan/INDEX.md\n"
            "2. Create docs/plan/{name}/context.yaml (see docs/plan/README.md for template)"
        )
        sys.exit(0)

    # Deduplicate by dir_name (keep first occurrence)
    seen_dirs = set()
    unique_entries = []
    for entry in entries:
        if entry["dir_name"] not in seen_dirs:
            seen_dirs.add(entry["dir_name"])
            unique_entries.append(entry)
    entries = unique_entries

    # Filter to plans with context.yaml
    candidates = []
    for entry in entries:
        plan_dir = os.path.join(args.plan_root, entry["dir_name"])
        context_path = os.path.join(plan_dir, "context.yaml")
        if os.path.isfile(context_path):
            entry["has_manifest"] = True
            entry["plan_dir"] = plan_dir

            # Read manifest metadata
            targets, manifest_status = load_target_metadata(context_path)
            entry["targets"] = targets
            entry["manifest_status"] = manifest_status

            # Count checklist progress
            progress = count_checklist_progress(plan_dir)
            entry["progress"] = progress

            candidates.append(entry)

    if not candidates:
        print(
            "No plans with context.yaml found in Active/Draft status.\n"
            "To make a plan available for /implement:\n"
            "1. Ensure the plan is registered in docs/plan/INDEX.md\n"
            "2. Create docs/plan/{name}/context.yaml (see docs/plan/README.md for template)"
        )
        sys.exit(0)

    # Recommend only the latest plans by index date (YYYY-MM-DD).
    candidates = sorted(
        candidates,
        key=lambda c: (c["date"], c["dir_name"]),
        reverse=True,
    )
    recommended = candidates[: args.max_candidates]

    total_contexts = count_all_contexts(args.plan_root)

    # Output numbered list with reasons
    print("Available plans for /implement (latest recommendations):\n")
    for i, c in enumerate(recommended, 1):
        reasons = []
        reasons.append(f"Status: {c['status']}")
        reasons.append(f"Updated: {c['date']}")
        if c["progress"]:
            done, tot = c["progress"]
            pct = int(done / tot * 100) if tot > 0 else 0
            reasons.append(f"Progress: {done}/{tot} ({pct}%)")
        reasons.append(f"Manifest: {c['manifest_status']}")
        reasons.append(f"Targets: {', '.join(c['targets'])}")

        print(f"  {i}. {c['name']} ({c['dir_name']})")
        print(f"     {' | '.join(reasons)}")
        print()

    print(
        f"Summary: recommended={len(recommended)} (latest by Updated date, max={args.max_candidates}) "
        f"| candidates={len(candidates)} (Active/Draft in INDEX) "
        f"| all_contexts={total_contexts} (docs/plan/*/context.yaml on disk)"
    )
    print(f"Enter a number (1-{len(recommended)}) to select a plan.")


if __name__ == "__main__":
    main()
