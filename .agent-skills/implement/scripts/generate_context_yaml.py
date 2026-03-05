#!/usr/bin/env python3
"""Generate context.yaml files for plan directories.

Reads docs/plan/INDEX.md to determine Active + Draft plan directories,
scans each for plan/checklist files, and reconciles context.yaml.

Usage:
    python3 generate_context_yaml.py --plan-root <path> [--docs-root <path>] [--dry-run] [--write]

Flags:
    --plan-root   Path to docs/plan/ directory
    --docs-root   Path to docs/ directory (optional, auto-detected if omitted)
    --dry-run     Print generated YAML to stdout (default)
    --write       Actually write context.yaml files
"""

import argparse
import os
import re
import sys

LINK_PATTERN = re.compile(r"\[[^\]]+\]\(([^)]+)\)")


def normalize_target_config(config: dict) -> dict:
    """Normalize target config for deterministic output.

    - references: deduplicate (first occurrence wins), exclude first-class field paths, then sort
    """
    targets = config.get("targets", {})
    for target_name, target in targets.items():
        refs = target.get("references")
        if not refs:
            continue

        # Collect first-class field paths to exclude from references
        first_class_paths = {
            target.get("plan", ""),
            target.get("checklist", ""),
            target.get("spec", ""),
            target.get("testPlan", ""),
            target.get("testChecklist", ""),
        }
        first_class_paths.discard("")

        seen = set()
        normalized = []
        for ref_path in refs:
            if not ref_path or ref_path in seen or ref_path in first_class_paths:
                continue
            seen.add(ref_path)
            normalized.append(ref_path)

        normalized.sort()
        if normalized:
            targets[target_name]["references"] = normalized
        else:
            targets[target_name].pop("references", None)

    return config


def infer_reference_type(rel_path: str) -> str:
    """Infer reference type from relative path."""
    if rel_path.startswith("../../spec/"):
        return "spec"
    if rel_path.startswith("../../reports/"):
        return "report"
    if rel_path.startswith("../../discuss/"):
        return "discuss"

    name = os.path.splitext(os.path.basename(rel_path))[0]
    if name == "implementation":
        return "implementation-plan"
    if name == "unit-test-plan":
        return "unit-test-plan"
    if name == "frontend-test-plan":
        return "frontend-test-plan"
    if name == "e2e-test-plan":
        return "e2e-test-plan"
    return name


def to_plan_relative_path(plan_dir_path: str, abs_path: str) -> str:
    """Convert absolute path to normalized relative path from plan dir."""
    rel_path = os.path.relpath(abs_path, plan_dir_path).replace("\\", "/")
    if not rel_path.startswith("."):
        rel_path = f"./{rel_path}"
    return rel_path


def collect_association_links(
    plan_dir_path: str,
    docs_root: str,
    doc_rel_paths: list[str],
    excluded_rel_paths: set[str],
) -> list[dict]:
    """Collect references from document header association lines.

    Scans lines likely used for linkage:
    - lines containing "关联"
    - lines containing "相关文档导航"
    - blockquote list lines starting with "> - "
    """
    refs = []
    abs_docs_root = os.path.abspath(docs_root)
    for doc_rel in doc_rel_paths:
        doc_abs = os.path.abspath(os.path.join(plan_dir_path, doc_rel))
        if not os.path.isfile(doc_abs):
            continue

        with open(doc_abs, "r", encoding="utf-8") as f:
            lines = f.readlines()

        for line in lines:
            stripped = line.strip()
            if (
                "关联" not in line
                and "相关文档导航" not in line
                and not stripped.startswith("> - ")
            ):
                continue

            for link in LINK_PATTERN.findall(line):
                if (
                    link.startswith("http://")
                    or link.startswith("https://")
                    or link.startswith("#")
                ):
                    continue

                resolved = os.path.abspath(os.path.join(os.path.dirname(doc_abs), link))
                if not resolved.endswith(".md"):
                    continue
                if not (
                    resolved.startswith(abs_docs_root + os.sep) or resolved == abs_docs_root
                ):
                    continue
                if not os.path.isfile(resolved):
                    continue

                rel_path = to_plan_relative_path(plan_dir_path, resolved)
                if rel_path in excluded_rel_paths:
                    continue

                refs.append(rel_path)

    return refs


def parse_index_plans(index_path: str) -> list[str]:
    """Parse INDEX.md and extract plan directory names from ALL sections.

    Returns a deduplicated list of directory names.
    """
    if not os.path.isfile(index_path):
        print(f"ERROR: INDEX.md not found: {index_path}", file=sys.stderr)
        sys.exit(1)

    with open(index_path, "r", encoding="utf-8") as f:
        content = f.read()

    plan_dirs = set()
    for line in content.split("\n"):
        # Extract directory references from table rows like:
        # | [name](./dir-name/) | ...
        # | ... [file](./dir-name/file.md) ...
        dir_matches = re.findall(r"\./([a-zA-Z0-9_-]+)/", line)
        for d in dir_matches:
            plan_dirs.add(d)

    return sorted(plan_dirs)


def scan_all_plan_dirs(plan_root: str, index_dirs: list[str]) -> list[str]:
    """Scan filesystem for plan directories not listed in INDEX.md.

    Combines INDEX entries with any directories found on disk that contain
    at least one .md file (to avoid picking up stale empty dirs).
    """
    all_dirs = set(index_dirs)
    if os.path.isdir(plan_root):
        for entry in os.listdir(plan_root):
            entry_path = os.path.join(plan_root, entry)
            if not os.path.isdir(entry_path):
                continue
            # Skip non-plan dirs (README.md, INDEX.md are files, not dirs)
            md_files = [f for f in os.listdir(entry_path) if f.endswith(".md")]
            if md_files:
                all_dirs.add(entry)
    return sorted(all_dirs)


def find_spec_file(spec_dir: str, dir_name: str) -> str | None:
    """Find a matching spec file for a plan directory.

    Tries patterns like:
    - {dir_name}-design.md
    - {dir_name}.md
    """
    candidates = [
        f"{dir_name}-design.md",
        f"{dir_name}.md",
    ]
    for candidate in candidates:
        path = os.path.join(spec_dir, candidate)
        if os.path.isfile(path):
            return candidate
    return None


def scan_directory_targets(
    plan_dir_path: str, dir_name: str, spec_dir: str, docs_root: str
) -> dict:
    """Scan a plan directory and determine targets.

    Returns a dict with:
        - defaultTarget: str
        - targets: dict of target_name -> target_config
    """
    if not os.path.isdir(plan_dir_path):
        return None

    files = set(os.listdir(plan_dir_path))
    targets = {}

    spec_file = find_spec_file(spec_dir, dir_name)

    # --- Standard targets ---

    # backend: implementation.md + implementation-checklist.md
    if "implementation.md" in files and "implementation-checklist.md" in files:
        target = {
            "plan": "./implementation.md",
            "checklist": "./implementation-checklist.md",
        }
        if spec_file:
            target["spec"] = f"../../spec/{spec_file}"
        # Promote unit test plan/checklist to first-class fields
        if "unit-test-plan.md" in files:
            target["testPlan"] = "./unit-test-plan.md"
        if "unit-test-plan-checklist.md" in files:
            target["testChecklist"] = "./unit-test-plan-checklist.md"
        targets["backend"] = target

    # Special: implementation.md + checklist.md (non-standard checklist name)
    elif "implementation.md" in files and "checklist.md" in files:
        target = {
            "plan": "./implementation.md",
            "checklist": "./checklist.md",
        }
        if spec_file:
            target["spec"] = f"../../spec/{spec_file}"
        refs = []
        # Check for design-improvements.md
        if "design-improvements.md" in files:
            refs.append("./design-improvements.md")
        if refs:
            target["references"] = refs
        targets["backend"] = target

    # Special: implementation.md exists but no checklist at all
    elif "implementation.md" in files:
        # Some directories have implementation.md without a matching checklist
        # (e.g., clustermeta). We still create a backend target referencing
        # what we can; the checklist field will reference the implementation
        # itself as a fallback - but let's skip these or handle per case.
        # Actually, look for any *-checklist.md that could pair
        pass

    # frontend: frontend-implementation.md + frontend-implementation-checklist.md
    if (
        "frontend-implementation.md" in files
        and "frontend-implementation-checklist.md" in files
    ):
        target = {
            "plan": "./frontend-implementation.md",
            "checklist": "./frontend-implementation-checklist.md",
        }
        if spec_file:
            target["spec"] = f"../../spec/{spec_file}"
        # Promote frontend test plan/checklist to first-class fields
        if "frontend-test-plan.md" in files:
            target["testPlan"] = "./frontend-test-plan.md"
        if "frontend-test-plan-checklist.md" in files:
            target["testChecklist"] = "./frontend-test-plan-checklist.md"
        targets["frontend"] = target

    # unit-test: promoted to testPlan/testChecklist on backend target (no standalone target)

    # e2e-test: e2e-test-plan.md + e2e-test-plan-checklist.md
    if "e2e-test-plan.md" in files and "e2e-test-plan-checklist.md" in files:
        target = {
            "plan": "./e2e-test-plan.md",
            "checklist": "./e2e-test-plan-checklist.md",
        }
        if spec_file:
            target["spec"] = f"../../spec/{spec_file}"
        refs = []
        if "implementation.md" in files:
            refs.append("./implementation.md")
        if refs:
            target["references"] = refs
        targets["e2e-test"] = target

    # frontend-test: promoted to testPlan/testChecklist on frontend target (no standalone target)

    # --- Custom named targets ---
    # Scan for pairs like {name}.md + {name}-checklist.md that we haven't
    # already covered

    standard_plan_prefixes = {
        "implementation",
        "frontend-implementation",
        "unit-test-plan",
        "e2e-test-plan",
        "frontend-test-plan",
    }

    # Find all .md files that have a matching -checklist.md
    md_files = {f for f in files if f.endswith(".md")}
    for md_file in sorted(md_files):
        base = md_file[:-3]  # strip .md
        if base.endswith("-checklist"):
            continue  # skip checklist files
        if base in standard_plan_prefixes:
            continue  # already handled
        if base == "checklist":
            continue  # already handled as special case
        if base == "context":
            continue  # skip context.yaml

        # Try two checklist patterns:
        # 1. {base}-checklist.md  (e.g., refactor.md -> refactor-checklist.md)
        # 2. If base ends with "-plan", try {stem}-checklist.md
        #    (e.g., dashboard-navigation-plan.md -> dashboard-navigation-checklist.md)
        checklist_name = f"{base}-checklist.md"
        target_name = base
        if checklist_name not in files and base.endswith("-plan"):
            stem = base[: -len("-plan")]
            alt_checklist = f"{stem}-checklist.md"
            if alt_checklist in files:
                checklist_name = alt_checklist
                target_name = stem  # e.g., "dashboard-navigation"

        if checklist_name in files:
            # This is a custom plan pair
            if target_name not in targets:
                target = {
                    "plan": f"./{md_file}",
                    "checklist": f"./{checklist_name}",
                }
                if spec_file:
                    target["spec"] = f"../../spec/{spec_file}"
                targets[target_name] = target

    # --- Standalone files (no checklist pair) ---
    # e.g., acceptance.md, review-checklist.md, controller-webhook-plan.md
    standalone_targets = {}
    uncovered_standalone_files = []  # standalone plan docs to add as references

    for md_file in sorted(md_files):
        base = md_file[:-3]
        if base.endswith("-checklist"):
            # Check if there's a matching plan
            plan_base = base[: -len("-checklist")]
            has_plan = (
                f"{plan_base}.md" in files
                or f"{plan_base}-plan.md" in files
            )
            if not has_plan:
                # Standalone checklist (e.g., review-checklist.md, implementation-checklist.md alone)
                target_name = plan_base if plan_base != "implementation" else "backend"
                if target_name not in targets:
                    standalone_targets[target_name] = {
                        "plan": f"./{md_file}",
                        "checklist": f"./{md_file}",
                    }
            continue
        if base == "context":
            continue

        # Skip files already part of a target
        already_covered = False
        for t in targets.values():
            plan_path = t.get("plan", "")
            checklist_path = t.get("checklist", "")
            spec_path = t.get("spec", "")
            test_plan_path = t.get("testPlan", "")
            test_checklist_path = t.get("testChecklist", "")
            if f"./{md_file}" in (plan_path, checklist_path, spec_path, test_plan_path, test_checklist_path):
                already_covered = True
                break
            if f"./{md_file}" in t.get("references", []):
                already_covered = True
                break
        if already_covered:
            continue

        # Check if it has a checklist pair - if so, it was handled above
        if f"{base}-checklist.md" in files:
            continue
        # Also check {stem}-checklist.md for {stem}-plan.md pattern
        if base.endswith("-plan"):
            stem = base[: -len("-plan")]
            if f"{stem}-checklist.md" in files:
                continue

        # Truly standalone files - add as additional references to existing
        # targets or as standalone targets
        # Known standalone patterns:
        if base == "acceptance":
            target = {
                "plan": f"./{md_file}",
                "checklist": f"./{md_file}",
            }
            if spec_file:
                target["spec"] = f"../../spec/{spec_file}"
            standalone_targets["acceptance"] = target
        elif base == "review-checklist":
            target = {
                "plan": f"./{md_file}",
                "checklist": f"./{md_file}",
            }
            if spec_file:
                target["spec"] = f"../../spec/{spec_file}"
            standalone_targets["review"] = target
        elif base == "test-plan-checklist":
            # standalone checklist like system-info-endpoint/test-plan-checklist.md
            target = {
                "plan": f"./{md_file}",
                "checklist": f"./{md_file}",
            }
            if spec_file:
                target["spec"] = f"../../spec/{spec_file}"
            standalone_targets["test"] = target
        else:
            # Standalone doc files - track for adding as references
            uncovered_standalone_files.append(md_file)

    # Merge standalone targets (only if not already covered)
    for name, target in standalone_targets.items():
        if name not in targets:
            targets[name] = target

    # Add uncovered standalone files as references to the default target
    # e.g., controller-webhook-plan.md in events-and-metrics,
    # test-completion-plan.md in template, implementation.md without checklist
    if uncovered_standalone_files and targets:
        # Find the best target to attach references to
        default_key = (
            "backend" if "backend" in targets
            else "frontend" if "frontend" in targets
            else sorted(targets.keys())[0]
        )
        default_tgt = targets[default_key]
        if "references" not in default_tgt:
            default_tgt["references"] = []
        for sf in uncovered_standalone_files:
            default_tgt["references"].append(f"./{sf}")

    # Collect association links from each target's own plan/checklist header.
    for target_name, target in targets.items():
        # Exclude all first-class field paths from association refs
        excluded = {target["plan"], target["checklist"]}
        if "spec" in target:
            excluded.add(target["spec"])
        if "testPlan" in target:
            excluded.add(target["testPlan"])
        if "testChecklist" in target:
            excluded.add(target["testChecklist"])
        target_refs = collect_association_links(
            plan_dir_path=plan_dir_path,
            docs_root=docs_root,
            doc_rel_paths=[target["plan"], target["checklist"]],
            excluded_rel_paths=excluded,
        )
        if target_refs:
            target.setdefault("references", [])
            target["references"].extend(target_refs)

    # --- Determine defaultTarget ---
    if not targets:
        return None

    if "backend" in targets:
        default_target = "backend"
    elif "frontend" in targets:
        default_target = "frontend"
    else:
        # Pick the first target alphabetically
        default_target = sorted(targets.keys())[0]

    return {
        "defaultTarget": default_target,
        "targets": targets,
    }


def format_yaml(dir_name: str, config: dict) -> str:
    """Format the context.yaml content as YAML string.

    We write YAML manually to maintain consistent formatting without
    requiring PyYAML as a dependency.
    """
    lines = [
        "apiVersion: ferry.agent.context/v1alpha1",
        "kind: PlanContext",
        "metadata:",
        f"  name: {dir_name}",
        "spec:",
        f"  defaultTarget: {config['defaultTarget']}",
        "  targets:",
    ]

    # Output targets in sorted order for deterministic output.
    ordered = sorted(config["targets"].keys())

    for target_name in ordered:
        target = config["targets"][target_name]
        lines.append(f"    {target_name}:")
        lines.append(f"      plan: {target['plan']}")
        lines.append(f"      checklist: {target['checklist']}")
        if "spec" in target:
            lines.append(f"      spec: {target['spec']}")
        if "testPlan" in target:
            lines.append(f"      testPlan: {target['testPlan']}")
        if "testChecklist" in target:
            lines.append(f"      testChecklist: {target['testChecklist']}")
        if "references" in target and target["references"]:
            lines.append("      references:")
            for ref_path in target["references"]:
                lines.append(f"        - {ref_path}")

    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(
        description="Generate context.yaml for plan directories"
    )
    parser.add_argument(
        "--plan-root",
        required=True,
        help="Path to docs/plan/ directory",
    )
    parser.add_argument(
        "--docs-root",
        help="Path to docs/ directory (optional, default: parent directory of --plan-root)",
    )
    mode_group = parser.add_mutually_exclusive_group()
    mode_group.add_argument(
        "--dry-run",
        action="store_true",
        help="Preview mode: print generated YAML to stdout (default mode)",
    )
    mode_group.add_argument(
        "--write",
        action="store_true",
        help="Write context.yaml files to disk",
    )
    args = parser.parse_args()

    plan_root = os.path.abspath(args.plan_root)
    docs_root = (
        os.path.abspath(args.docs_root)
        if args.docs_root
        else os.path.dirname(plan_root)
    )
    spec_dir = os.path.join(docs_root, "spec")
    index_path = os.path.join(plan_root, "INDEX.md")
    write_mode = args.write

    # 1. Parse INDEX.md for all plan dirs, then scan filesystem for any unlisted dirs
    index_dirs = parse_index_plans(index_path)
    plan_dirs = scan_all_plan_dirs(plan_root, index_dirs)
    print(
        f"Found {len(plan_dirs)} plan directories ({len(index_dirs)} from INDEX + filesystem scan)."
    )
    print(f"Mode: {'write' if write_mode else 'dry-run'}\n")

    generated = 0
    created = 0
    updated = 0
    reconciled = 0
    skipped_no_targets = 0

    for dir_name in plan_dirs:
        plan_dir_path = os.path.join(plan_root, dir_name)

        # Skip if directory doesn't exist
        if not os.path.isdir(plan_dir_path):
            print(f"SKIP (no dir): {dir_name}/")
            continue

        # Scan and generate
        config = scan_directory_targets(plan_dir_path, dir_name, spec_dir, docs_root)
        if config is None or not config["targets"]:
            print(f"SKIP (no targets): {dir_name}/")
            skipped_no_targets += 1
            continue
        config = normalize_target_config(config)

        yaml_content = format_yaml(dir_name, config)
        generated += 1
        context_path = os.path.join(plan_dir_path, "context.yaml")

        if write_mode:
            old_content = None
            if os.path.isfile(context_path):
                with open(context_path, "r", encoding="utf-8") as f:
                    old_content = f.read()

            with open(context_path, "w", encoding="utf-8") as f:
                f.write(yaml_content)

            if old_content is None:
                created += 1
                print(f"CREATED: {dir_name}/context.yaml")
            elif old_content == yaml_content:
                reconciled += 1
                print(f"RECONCILED: {dir_name}/context.yaml (unchanged)")
            else:
                updated += 1
                print(f"UPDATED: {dir_name}/context.yaml")
        else:
            print(f"--- {dir_name}/context.yaml ---")
            print(yaml_content)

    if write_mode:
        print(
            f"\nSummary: generated={generated}, created={created}, "
            f"updated={updated}, reconciled={reconciled}, "
            f"skipped_no_targets={skipped_no_targets}"
        )
    else:
        print(
            f"\nSummary: generated={generated}, "
            f"skipped_no_targets={skipped_no_targets}"
        )

    # Post-run check: in write mode, verify all target directories now have context.yaml
    if not write_mode:
        return

    missing = []
    for dir_name in plan_dirs:
        plan_dir_path = os.path.join(plan_root, dir_name)
        context_path = os.path.join(plan_dir_path, "context.yaml")
        if os.path.isdir(plan_dir_path) and not os.path.isfile(context_path):
            missing.append(dir_name)

    if missing:
        print(f"\nWARNING: {len(missing)} directories still missing context.yaml:")
        for m in missing:
            print(f"  - {m}/")
        sys.exit(1)
    else:
        print(f"\nAll {len(plan_dirs)} plan directories have context.yaml.")


if __name__ == "__main__":
    main()
