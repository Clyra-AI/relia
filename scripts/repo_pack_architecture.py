#!/usr/bin/env python3
from pathlib import Path
from datetime import datetime, timezone
import json
import os
import sys

ROOT = Path(__file__).resolve().parents[1]
FACTORYD_REPO_KEY = "relia"
ARCHITECTURE_BUDGET_EXCEPTION_REFS = [
    ".factory/artifacts/exceptions/architecture-debt-relia-main.json",
]
ARCHITECTURE_BUDGET_EXCEPTION_PATHS = [
    "cmd/relia/main.go",
    "cmd/relia/main_test.go",
]
FACTORY_ARCHITECTURE_BUDGET_POLICY_REF = "https://github.com/Clyra-AI/factory/blob/main/docs/standards/architecture-fitness-standard.md#default-budget"
EXPECTED_ARCHITECTURE_BUDGET_EXTENSIONS = [".go", ".py", ".ts", ".tsx", ".js", ".jsx"]
EXPECTED_ARCHITECTURE_BUDGET_EXCLUDED_DIRS = [
    ".git",
    ".factoryd",
    ".factory/tmp",
    ".relia",
    "workspaces",
    ".venv",
    "__pycache__",
    "build",
    "dist",
    "node_modules",
    "vendor",
]
ARCHITECTURE_BUDGET_COMPONENT_EXCLUDED_DIRS = {
    ".git",
    ".factoryd",
    ".venv",
    "__pycache__",
    "node_modules",
    "vendor",
}


def fail(message):
    print(message, file=sys.stderr)
    raise SystemExit(1)

def load_json_file(path):
    if not path.exists():
        fail(f"missing JSON artifact: {path.relative_to(ROOT)}")
    try:
        payload = json.loads(path.read_text())
    except Exception as exc:
        fail(f"{path.relative_to(ROOT)} is not valid JSON: {exc}")
    if not isinstance(payload, dict):
        fail(f"{path.relative_to(ROOT)} must contain a JSON object")
    return payload

def parse_rfc3339_timestamp(value, label):
    if not isinstance(value, str) or not value.strip():
        fail(f"{label} must be a non-empty RFC3339 timestamp")
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        fail(f"{label} must be RFC3339: {exc}")
    if parsed.tzinfo is None:
        fail(f"{label} must include a timezone")
    return parsed.astimezone(timezone.utc)

def architecture_debt_exception_expiry_error(ref, exception, now=None):
    expires_at = parse_rfc3339_timestamp(exception.get("expires_at"), f"{ref}.expires_at")
    reference_time = now or datetime.now(timezone.utc)
    if expires_at <= reference_time.astimezone(timezone.utc):
        return f"{ref}.expires_at must be in the future"
    return None

def validate_architecture_debt_exception_expiry(ref, exception, now=None):
    error = architecture_debt_exception_expiry_error(ref, exception, now)
    if error:
        fail(error)

def architecture_debt_exception_repo_ref_error(root, ref, key, values):
    if not isinstance(values, list) or not values:
        return f"{ref}.{key} must be a non-empty list"
    resolved_root = root.resolve()
    for index, value in enumerate(values):
        if not isinstance(value, str):
            return f"{ref}.{key}[{index}] must be a repo-local path string"
        if Path(value.strip()).is_absolute():
            return f"{ref}.{key}[{index}] must be a repo-local path"
        normalized = normalize_architecture_budget_path(value)
        if not normalized or normalized == ".." or normalized.startswith("../"):
            return f"{ref}.{key}[{index}] must be a repo-local path"
        resolved = (root / normalized).resolve()
        try:
            resolved.relative_to(resolved_root)
        except ValueError:
            return f"{ref}.{key}[{index}] must stay inside the repository"
        if not resolved.exists():
            return f"{ref}.{key}[{index}] points to missing repo path {normalized}"
    return None

def architecture_budget_policy_ref_error(root, value, label):
    if not isinstance(value, str) or not value.strip():
        return f"{label}.architecture_budget.policy_ref must be a non-empty string"
    policy_ref = value.strip()
    if policy_ref == FACTORY_ARCHITECTURE_BUDGET_POLICY_REF:
        return None
    if "#default-budget" not in policy_ref:
        return f"{label}.architecture_budget.policy_ref must cite the Factory architecture fitness default budget"
    path_ref = policy_ref.split("#", 1)[0]
    return architecture_debt_exception_repo_ref_error(root, label, "architecture_budget.policy_ref", [path_ref])

def architecture_debt_exception_line_ceiling_error(root, ref, scope):
    line_ceilings = scope.get("line_ceilings")
    if not isinstance(line_ceilings, dict):
        return f"{ref}.scope.line_ceilings must be an object"
    normalized_paths = [normalize_architecture_budget_path(path) for path in ARCHITECTURE_BUDGET_EXCEPTION_PATHS]
    if sorted(line_ceilings.keys()) != sorted(normalized_paths):
        return f"{ref}.scope.line_ceilings must cover {ARCHITECTURE_BUDGET_EXCEPTION_PATHS!r}"
    for path in normalized_paths:
        ceiling = line_ceilings.get(path)
        if type(ceiling) is not int or ceiling <= 0:
            return f"{ref}.scope.line_ceilings[{path!r}] must be a positive integer"
        source = root / path
        if not source.exists():
            return f"{ref}.scope.line_ceilings[{path!r}] points to missing source"
        line_count = count_file_lines(source)
        if ceiling != line_count:
            return f"{ref}.scope.line_ceilings[{path!r}] must equal current line count {line_count}"
    return None

def validate_architecture_debt_exception(ref):
    exception = load_json_file(ROOT / ref)
    if exception.get("artifact_type") != "architecture_debt_exception":
        fail(f"{ref} artifact_type must be architecture_debt_exception")
    for key in [
        "exception_id",
        "repo",
        "scope",
        "reason",
        "owner",
        "approved_by",
        "approved_at",
        "expires_at",
        "compensating_validation",
        "follow_up_refs",
        "evidence_refs",
    ]:
        if key not in exception:
            fail(f"{ref} missing {key}")
    if exception.get("repo") != FACTORYD_REPO_KEY:
        fail(f"{ref}.repo must be {FACTORYD_REPO_KEY}")
    scope = exception.get("scope")
    if not isinstance(scope, dict):
        fail(f"{ref}.scope must be an object")
    paths = scope.get("paths")
    if sorted(paths or []) != sorted(ARCHITECTURE_BUDGET_EXCEPTION_PATHS):
        fail(f"{ref}.scope.paths must be {ARCHITECTURE_BUDGET_EXCEPTION_PATHS!r}")
    line_ceiling_error = architecture_debt_exception_line_ceiling_error(ROOT, ref, scope)
    if line_ceiling_error:
        fail(line_ceiling_error)
    validate_architecture_debt_exception_expiry(ref, exception)
    if not isinstance(exception.get("compensating_validation"), list) or "make prepush-full" not in exception["compensating_validation"]:
        fail(f"{ref}.compensating_validation must include make prepush-full")
    for key in ["follow_up_refs", "evidence_refs"]:
        ref_error = architecture_debt_exception_repo_ref_error(ROOT, ref, key, exception.get(key))
        if ref_error:
            fail(ref_error)

def normalize_architecture_budget_path(value):
    path = str(value).strip().replace("\\", "/")
    while path.startswith("./"):
        path = path[2:]
    return path.strip("/")

def architecture_budget_path_excluded(rel, excluded_dirs):
    rel = normalize_architecture_budget_path(rel)
    if not rel:
        return False
    for part in rel.split("/"):
        if part in excluded_dirs and part in ARCHITECTURE_BUDGET_COMPONENT_EXCLUDED_DIRS:
            return True
    for excluded in excluded_dirs:
        if rel == excluded or rel.startswith(f"{excluded}/"):
            return True
    return False

def architecture_budget_exception_scope(root):
    approved = set()
    line_ceilings = {}
    for ref in ARCHITECTURE_BUDGET_EXCEPTION_REFS:
        exception = load_json_file(root / ref)
        scope = exception.get("scope") or {}
        scoped_line_ceilings = scope.get("line_ceilings") or {}
        for path in scope.get("paths") or []:
            normalized = normalize_architecture_budget_path(path)
            if normalized:
                approved.add(normalized)
                line_ceilings[normalized] = scoped_line_ceilings.get(normalized)
    return approved, line_ceilings

def count_file_lines(path):
    count = 0
    last = b""
    with path.open("rb") as handle:
        while True:
            chunk = handle.read(1024 * 1024)
            if not chunk:
                break
            count += chunk.count(b"\n")
            last = chunk[-1:]
    if last and last != b"\n":
        count += 1
    return count

def architecture_budget_unexcepted_failures(root, budget, exception_paths, exception_line_ceilings):
    extensions = {
        str(ext).strip().lower()
        for ext in budget.get("source_extensions") or []
        if str(ext).strip()
    }
    excluded_dirs = {
        normalize_architecture_budget_path(path)
        for path in budget.get("excluded_dirs") or []
        if normalize_architecture_budget_path(path)
    }
    fail_threshold = int(budget.get("fail_line_threshold") or 0)
    failures = []
    for dirpath, dirnames, filenames in os.walk(root):
        rel_dir = Path(dirpath).relative_to(root).as_posix()
        rel_dir = "" if rel_dir == "." else rel_dir
        dirnames[:] = [
            dirname
            for dirname in dirnames
            if not architecture_budget_path_excluded(str(Path(rel_dir) / dirname), excluded_dirs)
        ]
        for filename in filenames:
            path = Path(dirpath) / filename
            rel = path.relative_to(root).as_posix()
            if architecture_budget_path_excluded(rel, excluded_dirs):
                continue
            if path.suffix.lower() not in extensions:
                continue
            try:
                line_count = count_file_lines(path)
            except OSError as exc:
                failures.append(f"{rel} (unreadable: {exc})")
                continue
            if line_count < fail_threshold:
                continue
            if rel in exception_paths:
                ceiling = exception_line_ceilings.get(rel)
                if ceiling is None:
                    failures.append(f"{rel} is excepted but has no approved line ceiling")
                elif line_count > ceiling:
                    failures.append(f"{rel} ({line_count} lines > approved ceiling {ceiling})")
                continue
            failures.append(f"{rel} ({line_count} lines >= {fail_threshold})")
    return sorted(failures)

def validate_architecture_budget_inventory(budget, label):
    exception_paths, exception_line_ceilings = architecture_budget_exception_scope(ROOT)
    failures = architecture_budget_unexcepted_failures(
        ROOT,
        budget,
        exception_paths,
        exception_line_ceilings,
    )
    if failures:
        fail(f"{label}.architecture_budget has unexcepted over-budget source files: {', '.join(failures)}")

def validate_architecture_budget_policy(repo, label):
    budget = repo.get("architecture_budget")
    if not isinstance(budget, dict):
        fail(f"{label}.architecture_budget must be an object")
    if budget.get("enabled") is not True:
        fail(f"{label}.architecture_budget.enabled must be true")
    if budget.get("warn_line_threshold") != 1200:
        fail(f"{label}.architecture_budget.warn_line_threshold must be 1200")
    if budget.get("fail_line_threshold") != 2500:
        fail(f"{label}.architecture_budget.fail_line_threshold must be 2500")
    policy_ref_error = architecture_budget_policy_ref_error(ROOT, budget.get("policy_ref"), label)
    if policy_ref_error:
        fail(policy_ref_error)
    extensions = budget.get("source_extensions")
    if sorted(extensions or []) != sorted(EXPECTED_ARCHITECTURE_BUDGET_EXTENSIONS):
        fail(f"{label}.architecture_budget.source_extensions must be {EXPECTED_ARCHITECTURE_BUDGET_EXTENSIONS!r}")
    excluded = budget.get("excluded_dirs")
    if sorted(excluded or []) != sorted(EXPECTED_ARCHITECTURE_BUDGET_EXCLUDED_DIRS):
        fail(f"{label}.architecture_budget.excluded_dirs must be {EXPECTED_ARCHITECTURE_BUDGET_EXCLUDED_DIRS!r}")
    exception_refs = budget.get("exception_refs")
    if sorted(exception_refs or []) != sorted(ARCHITECTURE_BUDGET_EXCEPTION_REFS):
        fail(f"{label}.architecture_budget.exception_refs must be {ARCHITECTURE_BUDGET_EXCEPTION_REFS!r}")
    for ref in ARCHITECTURE_BUDGET_EXCEPTION_REFS:
        validate_architecture_debt_exception(ref)
    validate_architecture_budget_inventory(budget, label)

