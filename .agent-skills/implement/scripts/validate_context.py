#!/usr/bin/env python3
"""Validate plan context.yaml manifests.

Usage:
    # Single context
    python3 validate_context.py --context docs/plan/local-auth/context.yaml
    python3 validate_context.py --context docs/plan/local-auth/context.yaml --target backend

    # Batch mode (default): validate docs/plan/*/context.yaml
    python3 validate_context.py

Exit codes:
    0 - Validation passed
    1 - Batch mode: one or more contexts failed
    2 - Schema/field validation failed
    3 - Declared file does not exist
    4 - Path escapes docs/ boundary
    5 - Target does not exist
"""

import argparse
import json
import os
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

REQUIRED_API_VERSION = "agent.context/v1alpha1"
REQUIRED_KIND = "PlanContext"


class ValidationError(Exception):
    """Structured validation error with exit code."""

    def __init__(self, code: int, lines: list[str]):
        super().__init__("\n".join(lines))
        self.code = code
        self.lines = lines


def uniq_preserve_order(items: list[str]) -> list[str]:
    """Deduplicate while preserving order."""
    out = []
    seen = set()
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        out.append(item)
    return out


def load_manifest(context_path: str) -> dict:
    """Load and parse YAML manifest."""
    if not os.path.isfile(context_path):
        raise ValidationError(2, [f"ERROR: Manifest not found: {context_path}"])

    with open(context_path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    if not isinstance(data, dict):
        raise ValidationError(2, ["ERROR: Manifest must be a YAML mapping."])
    return data


def validate_schema(data: dict) -> list[str]:
    """Validate required fields and structure. Returns list of errors."""
    errors = []

    api_version = data.get("apiVersion")
    if api_version != REQUIRED_API_VERSION:
        errors.append(
            f"apiVersion must be '{REQUIRED_API_VERSION}', got '{api_version}'"
        )

    kind = data.get("kind")
    if kind != REQUIRED_KIND:
        errors.append(f"kind must be '{REQUIRED_KIND}', got '{kind}'")

    metadata = data.get("metadata")
    if not isinstance(metadata, dict) or not metadata.get("name"):
        errors.append("metadata.name is required")

    spec = data.get("spec")
    if not isinstance(spec, dict):
        errors.append("spec is required and must be a mapping")
        return errors

    default_target = spec.get("defaultTarget")
    if not default_target:
        errors.append("spec.defaultTarget is required")

    targets = spec.get("targets")
    if not isinstance(targets, dict) or len(targets) == 0:
        errors.append("spec.targets is required and must contain at least one target")
        return errors

    if default_target and default_target not in targets:
        errors.append(
            f"spec.defaultTarget '{default_target}' is not defined in spec.targets. "
            f"Available targets: {', '.join(sorted(targets.keys()))}"
        )

    for name, target in targets.items():
        if not isinstance(target, dict):
            errors.append(f"spec.targets.{name} must be a mapping")
            continue
        if not target.get("plan"):
            errors.append(f"spec.targets.{name}.plan is required")
        if not target.get("checklist"):
            errors.append(f"spec.targets.{name}.checklist is required")

        spec = target.get("spec")
        if spec is not None:
            if not isinstance(spec, str):
                errors.append(f"spec.targets.{name}.spec must be a string")
            elif not spec.endswith(".md"):
                errors.append(f"spec.targets.{name}.spec must end with .md")

        test_plan = target.get("testPlan")
        if test_plan is not None:
            if not isinstance(test_plan, str):
                errors.append(f"spec.targets.{name}.testPlan must be a string")
            elif not test_plan.endswith(".md"):
                errors.append(f"spec.targets.{name}.testPlan must end with .md")

        test_checklist = target.get("testChecklist")
        if test_checklist is not None:
            if not isinstance(test_checklist, str):
                errors.append(f"spec.targets.{name}.testChecklist must be a string")
            elif not test_checklist.endswith(".md"):
                errors.append(f"spec.targets.{name}.testChecklist must end with .md")

        refs = target.get("references")
        if refs is None:
            continue
        if not isinstance(refs, list):
            errors.append(f"spec.targets.{name}.references must be a list")
            continue
        for i, ref in enumerate(refs):
            if not isinstance(ref, str):
                errors.append(f"spec.targets.{name}.references[{i}] must be a string path")
            elif not ref.endswith(".md"):
                errors.append(f"spec.targets.{name}.references[{i}] must end with .md")

    return errors


def resolve_path(plan_dir: str, rel_path: str) -> str:
    """Resolve a relative path from plan directory and normalize."""
    return os.path.normpath(os.path.join(plan_dir, rel_path))


def check_path_boundary(resolved: str, docs_root: str) -> bool:
    """Check whether resolved path is inside docs root."""
    abs_resolved = os.path.abspath(resolved)
    abs_docs = os.path.abspath(docs_root)
    try:
        return os.path.commonpath([abs_resolved, abs_docs]) == abs_docs
    except ValueError:
        return False


def collect_target_files(
    target_data: dict,
    plan_dir: str,
    docs_root: str,
) -> tuple[list[dict], list[str], list[str], list[str]]:
    """Collect and validate files for one target.

    Returns:
        (file_list, missing_files, boundary_violations, non_markdown_files)
    """
    files = []
    missing = []
    boundary_errors = []
    non_markdown = []

    plan_rel = target_data["plan"]
    plan_path = resolve_path(plan_dir, plan_rel)
    files.append({"role": "plan", "path": plan_path})
    if not plan_path.endswith(".md"):
        non_markdown.append(
            f"  - {plan_rel} resolves to {plan_path} (expected *.md)"
        )
    elif not check_path_boundary(plan_path, docs_root):
        boundary_errors.append(
            f"  - {plan_rel} resolves to {plan_path} which is outside {docs_root}/"
        )
    elif not os.path.isfile(plan_path):
        missing.append(f"  - {plan_path}")

    checklist_rel = target_data["checklist"]
    checklist_path = resolve_path(plan_dir, checklist_rel)
    files.append({"role": "checklist", "path": checklist_path})
    if not checklist_path.endswith(".md"):
        non_markdown.append(
            f"  - {checklist_rel} resolves to {checklist_path} (expected *.md)"
        )
    elif not check_path_boundary(checklist_path, docs_root):
        boundary_errors.append(
            f"  - {checklist_rel} resolves to {checklist_path} which is outside {docs_root}/"
        )
    elif not os.path.isfile(checklist_path):
        missing.append(f"  - {checklist_path}")

    spec_rel = target_data.get("spec")
    if spec_rel:
        spec_path = resolve_path(plan_dir, spec_rel)
        files.append({"role": "spec", "path": spec_path})
        if not spec_path.endswith(".md"):
            non_markdown.append(
                f"  - {spec_rel} resolves to {spec_path} (expected *.md)"
            )
        elif not check_path_boundary(spec_path, docs_root):
            boundary_errors.append(
                f"  - {spec_rel} resolves to {spec_path} which is outside {docs_root}/"
            )
        elif not os.path.isfile(spec_path):
            missing.append(f"  - {spec_path}")

    test_plan_rel = target_data.get("testPlan")
    if test_plan_rel:
        test_plan_path = resolve_path(plan_dir, test_plan_rel)
        files.append({"role": "test-plan", "path": test_plan_path})
        if not test_plan_path.endswith(".md"):
            non_markdown.append(
                f"  - {test_plan_rel} resolves to {test_plan_path} (expected *.md)"
            )
        elif not check_path_boundary(test_plan_path, docs_root):
            boundary_errors.append(
                f"  - {test_plan_rel} resolves to {test_plan_path} which is outside {docs_root}/"
            )
        elif not os.path.isfile(test_plan_path):
            missing.append(f"  - {test_plan_path}")

    test_checklist_rel = target_data.get("testChecklist")
    if test_checklist_rel:
        test_checklist_path = resolve_path(plan_dir, test_checklist_rel)
        files.append({"role": "test-checklist", "path": test_checklist_path})
        if not test_checklist_path.endswith(".md"):
            non_markdown.append(
                f"  - {test_checklist_rel} resolves to {test_checklist_path} (expected *.md)"
            )
        elif not check_path_boundary(test_checklist_path, docs_root):
            boundary_errors.append(
                f"  - {test_checklist_rel} resolves to {test_checklist_path} which is outside {docs_root}/"
            )
        elif not os.path.isfile(test_checklist_path):
            missing.append(f"  - {test_checklist_path}")

    for ref_rel in target_data.get("references", []):
        ref_path = resolve_path(plan_dir, ref_rel)
        files.append({"role": "reference", "path": ref_path})
        if not ref_path.endswith(".md"):
            non_markdown.append(
                f"  - {ref_rel} resolves to {ref_path} (expected *.md)"
            )
        elif not check_path_boundary(ref_path, docs_root):
            boundary_errors.append(
                f"  - {ref_rel} resolves to {ref_path} which is outside {docs_root}/"
            )
        elif not os.path.isfile(ref_path):
            missing.append(f"  - {ref_path}")

    return files, missing, boundary_errors, non_markdown


def normalize_docs_root(docs_root: str) -> str:
    """Normalize docs root input.

    If user passes docs/plan, auto-upcast to docs.
    """
    abs_root = os.path.abspath(docs_root)
    if os.path.basename(abs_root) == "plan":
        parent = os.path.dirname(abs_root)
        if os.path.isdir(abs_root):
            return parent
    return abs_root


def infer_docs_root(context_path: str | None, plan_root: str) -> str:
    """Infer docs root from context path or plan root."""
    if context_path:
        cursor = os.path.abspath(os.path.dirname(context_path))
        while True:
            if os.path.basename(cursor) == "plan":
                return os.path.dirname(cursor)
            parent = os.path.dirname(cursor)
            if parent == cursor:
                break
            cursor = parent

    abs_plan_root = os.path.abspath(plan_root)
    if os.path.basename(abs_plan_root) == "plan":
        return os.path.dirname(abs_plan_root)

    maybe_plan = os.path.join(abs_plan_root, "plan")
    if os.path.isdir(maybe_plan):
        return abs_plan_root

    return os.path.abspath("docs")


def collect_contexts(plan_root: str) -> list[str]:
    """Collect docs/plan/*/context.yaml under plan root."""
    abs_plan_root = os.path.abspath(plan_root)
    if not os.path.isdir(abs_plan_root):
        return []

    contexts = []
    for entry in sorted(os.listdir(abs_plan_root)):
        plan_dir = os.path.join(abs_plan_root, entry)
        if not os.path.isdir(plan_dir):
            continue
        context_path = os.path.join(plan_dir, "context.yaml")
        if os.path.isfile(context_path):
            contexts.append(context_path)
    return contexts


def validate_context(
    context_path: str,
    docs_root: str,
    target: str | None = None,
) -> dict:
    """Validate one context and return normalized output payload."""
    data = load_manifest(context_path)

    schema_errors = validate_schema(data)
    if schema_errors:
        raise ValidationError(
            2,
            ["ERROR: Schema validation failed:"]
            + [f"  - {err}" for err in schema_errors],
        )

    spec = data["spec"]
    targets = spec["targets"]
    plan_dir = os.path.dirname(os.path.abspath(context_path))

    if target:
        if target not in targets:
            raise ValidationError(
                5,
                [
                    f"ERROR: Target '{target}' not found.",
                    f"Available targets: {', '.join(sorted(targets.keys()))}",
                ],
            )

        files, missing, boundary, non_markdown = collect_target_files(
            targets[target], plan_dir, docs_root
        )
        non_markdown = uniq_preserve_order(non_markdown)
        boundary = uniq_preserve_order(boundary)
        missing = uniq_preserve_order(missing)

        if non_markdown:
            raise ValidationError(
                2,
                ["ERROR: Referenced file must be markdown (*.md):"] + non_markdown,
            )
        if boundary:
            raise ValidationError(
                4,
                ["ERROR: Path escapes docs/ boundary:"] + boundary,
            )
        if missing:
            raise ValidationError(
                3,
                ["ERROR: Referenced file does not exist:"] + missing,
            )

        return {
            "name": data["metadata"]["name"],
            "target": target,
            "defaultTarget": spec["defaultTarget"],
            "files": files,
        }

    all_boundary = []
    all_missing = []
    all_non_markdown = []
    for tdata in targets.values():
        _, missing, boundary, non_markdown = collect_target_files(
            tdata, plan_dir, docs_root
        )
        all_boundary.extend(boundary)
        all_missing.extend(missing)
        all_non_markdown.extend(non_markdown)

    all_non_markdown = uniq_preserve_order(all_non_markdown)
    all_boundary = uniq_preserve_order(all_boundary)
    all_missing = uniq_preserve_order(all_missing)

    if all_non_markdown:
        raise ValidationError(
            2,
            ["ERROR: Referenced file must be markdown (*.md):"] + all_non_markdown,
        )
    if all_boundary:
        raise ValidationError(
            4,
            ["ERROR: Path escapes docs/ boundary:"] + all_boundary,
        )
    if all_missing:
        raise ValidationError(
            3,
            ["ERROR: Referenced file does not exist:"] + all_missing,
        )

    return {
        "name": data["metadata"]["name"],
        "defaultTarget": spec["defaultTarget"],
        "targets": sorted(targets.keys()),
    }


def main():
    parser = argparse.ArgumentParser(description="Validate plan context.yaml manifest(s)")
    parser.add_argument(
        "--context",
        help="Path to a context.yaml manifest (single mode). If omitted, validate all under --plan-root.",
    )
    parser.add_argument(
        "--plan-root",
        default="docs/plan",
        help="Plan root directory for batch mode (default: docs/plan)",
    )
    parser.add_argument(
        "--docs-root",
        help="Documentation root directory (default: auto-detected as docs/)",
    )
    parser.add_argument(
        "--target",
        help="Optional target name (single mode only)",
    )
    args = parser.parse_args()

    if args.target and not args.context:
        print(
            "ERROR: --target requires --context (single context mode).",
            file=sys.stderr,
        )
        sys.exit(2)

    docs_root = (
        normalize_docs_root(args.docs_root)
        if args.docs_root
        else infer_docs_root(args.context, args.plan_root)
    )

    if args.context:
        try:
            out = validate_context(
                context_path=args.context,
                docs_root=docs_root,
                target=args.target,
            )
        except ValidationError as err:
            for line in err.lines:
                print(line, file=sys.stderr)
            sys.exit(err.code)

        print(json.dumps(out, ensure_ascii=False, indent=2))
        sys.exit(0)

    contexts = collect_contexts(args.plan_root)
    if not contexts:
        print(f"No context.yaml found under {args.plan_root}/*/context.yaml")
        sys.exit(0)

    failures = []
    validated = 0
    for context_path in contexts:
        rel = os.path.relpath(context_path)
        try:
            validate_context(context_path=context_path, docs_root=docs_root, target=None)
            print(f"OK: {rel}")
            validated += 1
        except ValidationError as err:
            failures.append((context_path, err))
            print(f"FAIL: {rel}", file=sys.stderr)
            for line in err.lines:
                print(line, file=sys.stderr)

    total = len(contexts)
    failed = len(failures)
    print(f"SUMMARY: total={total} passed={validated} failed={failed}")

    if failures:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
