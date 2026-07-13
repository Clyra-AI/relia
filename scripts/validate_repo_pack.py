#!/usr/bin/env python3
from pathlib import Path
from datetime import datetime, timezone
import json
import re
import sys
from repo_pack_architecture import validate_architecture_budget_policy

ROOT = Path(__file__).resolve().parents[1]
FACTORYD_CONFIG = ROOT / ".factory" / "factoryd.example.json"
FACTORYD_ACTIVE_CONFIG = ROOT / ".factory" / "factoryd.json"
FACTORYD_AUTOSHIP_CONFIG = ROOT / ".factory" / "factoryd.autoship.example.json"
FACTORYD_REPO_KEY = "relia"
SUPPORTED_CAPABILITY_GRANTS = {
    "approval",
    "merge_authority",
    "network",
    "credentials",
    "model_provider_endpoint",
    "model_artifact_pull",
    "model_route_override",
    "codex_review_carry_forward",
}
PRIVILEGED_CAPABILITY_GRANTS = SUPPORTED_CAPABILITY_GRANTS - {"approval"}
PROVIDER_ACCEPTANCE_IDS = {
    "FR23-PROVIDER-ADAPTERS-AND-NO-LLM-MODE-001",
    "MVP-IN-SCOPE-010",
    "MVP-IN-SCOPE-011",
    "DISTILL-REVIEW-MEMORY-PAGE-ACCEPTANCE-TESTS-010",
}
RUNNER_READY_TASK_FIELDS = [
    "task_id",
    "objective",
    "semantic_invariants",
    "allowed_paths",
    "forbidden_paths",
    "validation_commands",
    "baseline_commands",
    "red_first_commands",
    "final_validation_commands",
    "acceptance_result_requirements",
    "evidence_required",
    "worker_evidence_required",
    "lifecycle_evidence_required",
    "stop_conditions",
    "worker_type",
    "factoryd_runtime",
    "required_worker_chain",
    "lifecycle_gates",
    "test_matrix_refs",
    "ci_lane_refs",
    "ci_control_refs",
    "coverage_policy_refs",
    "security_scanner_gates",
    "engineering_policy_refs",
    "architecture_guidance_refs",
    "changelog_intent",
    "versioning_impact",
    "migration_impact",
    "docs_sync_refs",
    "acceptance_group_id",
    "acceptance_ledger_ref",
    "acceptance_item_ids",
    "alignment_gate_ref",
    "plan_drift_policy_ref",
    "required_proof_level",
    "artifact_budget_refs",
    "redaction_posture",
]
REQUIRED_PROOF_LEVELS = {
    "syntax",
    "source_evidence",
    "workflow_behavior",
    "user_visible_behavior",
}
LIFECYCLE_EVIDENCE_KEYS = {
    "factoryd_run_once_report",
    "post_merge_report",
    "pr_lifecycle_report",
    "scope_closure_map",
    "scope_closure_report",
    "ship_packet",
}
WORKER_EVIDENCE_KEYS = {
    "proof_of_behavior_scorecard",
    "validation_report",
    "work_proof_marker",
}

REQUIRED = [
    "AGENTS.md",
    "WORKFLOW.md",
    "README.md",
    "docs/product/prd.md",
    "docs/dev/dev_guides.md",
    "docs/architecture/architecture_guides.md",
    ".factory/README.md",
    ".factory/factoryd.example.json",
    ".factory/factoryd.autoship.example.json",
    ".factory/profile.yaml",
    ".factory/artifacts/supervisor-runs/.gitkeep",
    ".factory/artifacts/task-supervisor-runs/.gitkeep",
    ".github/required-checks.json",
    ".github/CODEOWNERS",
    ".github/action-ref-exceptions.yaml",
    ".github/workflows/validate.yml",
    ".github/workflows/codeql.yml",
    "scripts/check_go_coverage.py",
]
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

def repo_local_path(value, flag):
    path = Path(value)
    if not path.is_absolute():
        path = ROOT / path
    path = path.resolve()
    try:
        path.relative_to(ROOT)
    except ValueError:
        fail(f"{flag} must point at a repo-local path")
    return path

def parse_args(argv):
    options = {
        "self_test": False,
        "factoryd_config": FACTORYD_CONFIG,
    }
    index = 0
    while index < len(argv):
        arg = argv[index]
        if arg == "--self-test":
            options["self_test"] = True
            index += 1
            continue
        if arg == "--factoryd-config":
            index += 1
            if index >= len(argv) or not str(argv[index]).strip():
                fail("--factoryd-config requires a repo-local path")
            options["factoryd_config"] = repo_local_path(argv[index], "--factoryd-config")
            index += 1
            continue
        fail(f"unknown argument: {arg}")
    return options

def factoryd_config_capability_grants():
    grants = []
    if not FACTORYD_ACTIVE_CONFIG.exists():
        return grants
    config = load_json_file(FACTORYD_ACTIVE_CONFIG)
    repos = config.get("repos")
    if isinstance(repos, dict):
        repo = repos.get(FACTORYD_REPO_KEY)
        if isinstance(repo, dict) and isinstance(repo.get("capability_grants"), list):
            grants.extend(grant for grant in repo["capability_grants"] if isinstance(grant, dict))
    elif isinstance(config.get("capability_grants"), list):
        grants.extend(grant for grant in config["capability_grants"] if isinstance(grant, dict))
    return grants

def active_factoryd_repo_config(repo_key):
    if not FACTORYD_ACTIVE_CONFIG.exists():
        return None
    config = load_json_file(FACTORYD_ACTIVE_CONFIG)
    repos = config.get("repos")
    if not isinstance(repos, dict):
        if isinstance(config.get("capability_grants"), list):
            return None
        fail("active factoryd config must declare repos as an object")
    repo = repos.get(repo_key)
    if not isinstance(repo, dict):
        fail(f"active factoryd config missing repo key: {repo_key}")
    return repo

def validate_active_architecture_budget_policy(active_repo):
    if active_repo is None:
        return
    if "architecture_budget" in active_repo:
        validate_architecture_budget_policy(active_repo, "active factoryd config")
        return
    if active_repo_is_grant_overlay(active_repo):
        return
    fail("active factoryd config.architecture_budget must be an object for full active configs")

def validate_active_capability_grants(active_repo):
    if active_repo is None:
        if not FACTORYD_ACTIVE_CONFIG.exists():
            return
        active_config = load_json_file(FACTORYD_ACTIVE_CONFIG)
        if "capability_grants" not in active_config:
            return
        active_repo = {"capability_grants": active_config.get("capability_grants")}
    grants = active_repo.get("capability_grants")
    if not isinstance(grants, list):
        fail("active factoryd config capability_grants must be a list")
    for index, grant in enumerate(grants):
        label = f"active factoryd config capability_grants[{index}]"
        if not isinstance(grant, dict):
            fail(f"{label} must be an object")
        task_id = grant.get("task_id")
        if not isinstance(task_id, str) or not task_id.strip() or task_id != task_id.strip() or "\n" in task_id or "\r" in task_id:
            fail(f"{label}.task_id must be a canonical non-blank string")
        capability_value = grant.get("capability")
        capability = capability_value.strip().lower() if isinstance(capability_value, str) else ""
        if capability not in SUPPORTED_CAPABILITY_GRANTS:
            fail(f"{label}.capability is unsupported")
        if capability_value != capability:
            fail(f"{label}.capability must use its canonical lowercase value")
        if grant.get("approved") not in (True, False):
            fail(f"{label}.approved must be true or false")
        if grant.get("approved") is not True:
            continue
        if capability in PRIVILEGED_CAPABILITY_GRANTS and task_id == "*":
            fail(f"{label} approved {capability} grant requires a concrete task_id")
        evidence_ref = grant.get("evidence_ref")
        if not isinstance(evidence_ref, str) or not evidence_ref.strip():
            fail(f"{label} approved {capability} grant requires evidence_ref")
        expires_at = grant.get("expires_at")
        if expires_at not in (None, ""):
            if not isinstance(expires_at, str) or expires_at != expires_at.strip():
                fail(f"{label}.expires_at must be RFC3339")
            try:
                parsed_expiry = datetime.fromisoformat(expires_at.replace("Z", "+00:00"))
            except ValueError:
                fail(f"{label}.expires_at must be RFC3339")
            if parsed_expiry.tzinfo is None:
                fail(f"{label}.expires_at must be RFC3339")
            if parsed_expiry <= datetime.now(timezone.utc):
                fail(f"{label}.expires_at must be in the future")
        if capability == "merge_authority" and not expires_at:
            fail(f"{label} approved merge_authority grant requires expires_at")
        if capability == "network":
            allowlist = grant.get("network_allowlist")
            if not isinstance(allowlist, list) or not allowlist or not all(isinstance(item, str) and item.strip() for item in allowlist):
                fail(f"{label} approved network grant requires a concrete network_allowlist")
        if capability in {"credentials", "model_provider_endpoint"}:
            credential_environment = grant.get("credential_environment")
            if not isinstance(credential_environment, str) or not credential_environment.strip():
                fail(f"{label} approved {capability} grant requires credential_environment")
        if capability == "credentials":
            scopes = grant.get("credential_scopes")
            if not isinstance(scopes, list) or not scopes or not all(isinstance(item, str) and item.strip() for item in scopes):
                fail(f"{label} approved credentials grant requires credential_scopes")
        if capability == "model_provider_endpoint":
            required_provider_fields = [
                "provider_identity",
                "provider_model",
                "budget_posture",
                "redaction_posture",
            ]
            missing_provider_fields = [
                field
                for field in required_provider_fields
                if not isinstance(grant.get(field), str) or not grant[field].strip()
            ]
            if missing_provider_fields:
                fail(f"{label} approved model_provider_endpoint grant missing fields: {missing_provider_fields}")
            endpoint = grant.get("provider_endpoint") or grant.get("base_url")
            if not isinstance(endpoint, str) or not endpoint.strip():
                fail(f"{label} approved model_provider_endpoint grant requires provider_endpoint or base_url")
            allowlist = grant.get("network_allowlist")
            if not isinstance(allowlist, list) or not allowlist or not all(isinstance(item, str) and item.strip() for item in allowlist):
                fail(f"{label} approved model_provider_endpoint grant requires a concrete network_allowlist")
        if capability == "model_artifact_pull":
            required_artifact_fields = [
                "pull_command",
                "model_source_url",
                "model_id",
                "model_version",
                "model_license",
                "cache_path",
                "update_policy",
                "rollback_policy",
                "absence_behavior",
            ]
            missing_artifact_fields = [
                field
                for field in required_artifact_fields
                if not isinstance(grant.get(field), str) or not grant[field].strip()
            ]
            if missing_artifact_fields:
                fail(f"{label} approved model_artifact_pull grant missing fields: {missing_artifact_fields}")
            digest = grant.get("content_digest")
            if not isinstance(digest, str) or re.fullmatch(r"sha256:[0-9a-fA-F]{64}", digest.strip()) is None:
                fail(f"{label} approved model_artifact_pull grant requires a sha256 content_digest")
            allowlist = grant.get("network_allowlist")
            if not isinstance(allowlist, list) or not allowlist or not all(isinstance(item, str) and item.strip() for item in allowlist):
                fail(f"{label} approved model_artifact_pull grant requires a concrete network_allowlist")
        if capability in PRIVILEGED_CAPABILITY_GRANTS and not expires_at:
            fail(f"{label} approved {capability} grant requires expires_at")

def active_repo_is_grant_overlay(active_repo):
    overlay_keys = {
        "capability_grants",
        "metadata",
        "notes",
        "repo_key",
        "repo_name",
        "repo_path",
        "repo_root",
    }
    return isinstance(active_repo.get("capability_grants"), list) and set(active_repo.keys()) <= overlay_keys

def profile_visibility_from_text(profile_text):
    for line in profile_text.splitlines():
        stripped = line.strip()
        if stripped.startswith("repo_visibility:"):
            return stripped.split(":", 1)[1].strip().strip("'\"")
    fail("profile must declare repo_visibility")

def duplicate_values(values):
    seen = set()
    duplicates = []
    for value in values:
        if value in seen and value not in duplicates:
            duplicates.append(value)
        seen.add(value)
    return duplicates

def lifecycle_evidence_items(values):
    if not isinstance(values, list):
        return []
    return [
        item
        for item in values
        if str(item).strip().lower().replace("-", "_") in LIFECYCLE_EVIDENCE_KEYS
    ]

def non_lifecycle_evidence_items(values):
    if not isinstance(values, list):
        return []
    return [
        item
        for item in values
        if str(item).strip().lower().replace("-", "_") not in LIFECYCLE_EVIDENCE_KEYS
    ]

def worker_evidence_items(values):
    if not isinstance(values, list):
        return []
    return [
        item
        for item in values
        if str(item).strip().lower().replace("-", "_") in WORKER_EVIDENCE_KEYS
    ]

def non_worker_evidence_items(values):
    if not isinstance(values, list):
        return []
    return [
        item
        for item in values
        if str(item).strip().lower().replace("-", "_") not in WORKER_EVIDENCE_KEYS
    ]

def normalized_evidence_key(value):
    return str(value).strip().lower().replace("-", "_")

def require_worker_evidence_mirror(label, evidence_required, worker_evidence):
    evidence_defaults = {
        normalized_evidence_key(item)
        for item in worker_evidence_items(evidence_required)
    }
    worker_keys = {
        normalized_evidence_key(item)
        for item in worker_evidence_items(worker_evidence)
        if normalized_evidence_key(item)
    }
    if evidence_defaults and (not isinstance(worker_evidence, list) or not worker_evidence):
        fail(f"{label}.worker_evidence_required must mirror worker-owned evidence_required defaults: {sorted(evidence_defaults)!r}")
    if worker_keys and (not isinstance(evidence_required, list) or not evidence_required):
        fail(f"{label}.evidence_required must mirror worker-owned worker_evidence_required defaults: {sorted(worker_keys)!r}")
    missing_worker = sorted(evidence_defaults - worker_keys)
    if missing_worker:
        fail(f"{label}.worker_evidence_required must mirror worker-owned evidence_required defaults: {missing_worker!r}")
    missing_evidence = sorted(worker_keys - evidence_defaults)
    if missing_evidence:
        fail(f"{label}.evidence_required must mirror worker-owned worker_evidence_required defaults: {missing_evidence!r}")

def require_worker_values_present(field_label, required_worker_values, actual_values):
    required_defaults = {
        normalized_evidence_key(item)
        for item in worker_evidence_items(required_worker_values)
    }
    if not required_defaults:
        return
    if not isinstance(actual_values, list) or not actual_values:
        fail(f"{field_label} must mirror inherited worker-owned evidence defaults: {sorted(required_defaults)!r}")
    actual_keys = {
        normalized_evidence_key(item)
        for item in actual_values
        if normalized_evidence_key(item)
    }
    missing_worker = sorted(required_defaults - actual_keys)
    if missing_worker:
        fail(f"{field_label} must mirror inherited worker-owned evidence defaults: {missing_worker!r}")

def require_lifecycle_evidence_mirror(field_label, lifecycle_defaults, lifecycle_evidence):
    required_defaults = {
        normalized_evidence_key(item)
        for item in lifecycle_evidence_items(lifecycle_defaults)
    }
    if not required_defaults:
        return
    if not isinstance(lifecycle_evidence, list) or not lifecycle_evidence:
        fail(f"{field_label} must mirror lifecycle-owned evidence defaults: {sorted(required_defaults)!r}")
    lifecycle_keys = {
        normalized_evidence_key(item)
        for item in lifecycle_evidence
        if normalized_evidence_key(item)
    }
    missing_lifecycle = sorted(required_defaults - lifecycle_keys)
    if missing_lifecycle:
        fail(f"{field_label} must mirror lifecycle-owned evidence defaults: {missing_lifecycle!r}")

def normalize_repo_path(path):
    parts = []
    for part in str(path).replace("\\", "/").split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if parts:
                parts.pop()
            else:
                parts.append(part)
            continue
        parts.append(part)
    normalized = "/".join(parts)
    if str(path).endswith("/") and normalized:
        normalized += "/"
    return normalized

def path_matches_or_contains(path, root):
    path = normalize_repo_path(path).rstrip("/")
    root = normalize_repo_path(root).rstrip("/")
    return bool(path) and (path == root or path.startswith(root + "/"))

def path_is_ancestor_of(path, root):
    path = normalize_repo_path(path).rstrip("/")
    root = normalize_repo_path(root).rstrip("/")
    return bool(path) and path != root and root.startswith(path + "/")

def raw_repo_relative_path_error(path):
    if not isinstance(path, str):
        return "path must be a string"
    raw = str(path).strip()
    if not raw:
        return "path must be non-empty"
    if raw.startswith(("/", "\\")) or re.match(r"^[A-Za-z]:[\\/]", raw):
        return "path must be repo-relative, not absolute"
    if any(part == ".." for part in raw.replace("\\", "/").split("/")):
        return "path must not contain parent-directory segments"
    return ""

def normalized_scoped_repo_path(path, label):
    normalized = normalize_repo_path(path)
    if not normalized.rstrip("/"):
        fail(f"{label} path must not target repository root")
    return normalized

def validate_architecture_target_paths(task, task_id):
    targets = task.get("architecture_target_paths")
    if not isinstance(targets, list) or not targets:
        fail(f"{task_id}.architecture_target_paths must be a non-empty list")
    normalized_targets = []
    for index, path in enumerate(targets):
        path_error = raw_repo_relative_path_error(path)
        if path_error:
            fail(f"{task_id}.architecture_target_paths[{index}] {path_error}")
        normalized_targets.append(normalized_scoped_repo_path(path, f"{task_id}.architecture_target_paths[{index}]"))
    if any(path.rstrip("/") == "internal" for path in normalized_targets):
        fail(f"{task_id}.architecture_target_paths must name bounded internal packages, not broad internal/")
    method = task.get("path_planning_method")
    if not isinstance(method, str) or not method.strip():
        fail(f"{task_id}.path_planning_method must identify the architecture path-planning rule")
    allowed = []
    for index, path in enumerate(task.get("allowed_paths") or []):
        path_error = raw_repo_relative_path_error(path)
        if path_error:
            fail(f"{task_id}.allowed_paths[{index}] {path_error}")
        allowed.append(normalized_scoped_repo_path(path, f"{task_id}.allowed_paths[{index}]"))
    if any(path.rstrip("/") == "internal" for path in allowed):
        fail(f"{task_id}.allowed_paths must not include broad internal/ after decomposition")
    if any(path.rstrip("/") == "cmd" for path in allowed) and not any(path.rstrip("/") == "cmd" for path in normalized_targets):
        fail(f"{task_id}.allowed_paths may include cmd/ only when cmd/ is an explicit architecture target")
    missing = sorted(
        target for target in normalized_targets
        if not any(path_matches_or_contains(target, allowed_path) or path_is_ancestor_of(allowed_path, target) for allowed_path in allowed)
    )
    if missing:
        fail(f"{task_id}.allowed_paths must include architecture_target_paths: {missing}")

def validate_lifecycle_path_ownership(task, task_id):
    lifecycle_keys = {
        normalized_evidence_key(item)
        for item in task.get("lifecycle_evidence_required") or []
        if normalized_evidence_key(item)
    }
    inheritance = task.get("validation_contract_inheritance")
    if isinstance(inheritance, dict):
        lifecycle_keys.update(
            normalized_evidence_key(item)
            for item in inheritance.get("lifecycle_evidence_required") or []
            if normalized_evidence_key(item)
        )
    for requirement in task.get("acceptance_result_requirements") or []:
        if not isinstance(requirement, dict):
            continue
        lifecycle_keys.update(
            normalized_evidence_key(item)
            for item in requirement.get("lifecycle_evidence_required") or []
            if normalized_evidence_key(item)
        )
    if not lifecycle_keys:
        return
    allowed = {normalize_repo_path(path) for path in task.get("allowed_paths") or []}
    forbidden = {normalize_repo_path(path) for path in task.get("forbidden_paths") or []}
    work_item_id = str(task.get("work_item_id") or task_id).strip()
    task_runs_root = ".factory/artifacts/task-runs/"
    task_run_dir = f".factory/artifacts/task-runs/{task_id}/"
    pr_lifecycle_root = ".factory/artifacts/pr-lifecycle/"
    pr_lifecycle_dir = f".factory/artifacts/pr-lifecycle/{work_item_id}/"

    allowed_lifecycle_paths = sorted(
        path for path in allowed
        if path_matches_or_contains(path, pr_lifecycle_root)
        or path_is_ancestor_of(path, pr_lifecycle_root)
        or path_matches_or_contains(path, pr_lifecycle_dir)
        or path_is_ancestor_of(path, pr_lifecycle_dir)
        or (
            path_matches_or_contains(path, task_runs_root)
            and not path_matches_or_contains(path, task_run_dir)
        )
        or path_is_ancestor_of(path, task_runs_root)
    )
    if allowed_lifecycle_paths:
        fail(
            f"{task_id}.allowed_paths includes daemon-owned lifecycle evidence paths: "
            f"{allowed_lifecycle_paths}"
        )

    required_forbidden = []
    if "pr_lifecycle_report" in lifecycle_keys or "ship_packet" in lifecycle_keys or "post_merge_report" in lifecycle_keys:
        required_forbidden.append(pr_lifecycle_dir)
    if "scope_closure_report" in lifecycle_keys:
        required_forbidden.append(task_run_dir + "scope-closure-report.json")
    if "scope_closure_map" in lifecycle_keys or "scope_closure_report" in lifecycle_keys:
        required_forbidden.append(task_run_dir + "scope-closure-map.json")
    if "factoryd_run_once_report" in lifecycle_keys:
        required_forbidden.append(task_run_dir + "factoryd-run-once-report.json")

    missing_forbidden = sorted(path for path in required_forbidden if path not in forbidden)
    if missing_forbidden:
        fail(
            f"{task_id}.forbidden_paths must reserve daemon-owned lifecycle evidence paths: "
            f"{missing_forbidden}"
        )

def validate_validation_contract_evidence_split(contract, label):
    if not isinstance(contract, dict):
        fail(f"{label} must be an object")
    evidence_required = contract.get("evidence_required")
    worker_evidence = contract.get("worker_evidence_required")
    lifecycle_evidence = contract.get("lifecycle_evidence_required")
    if isinstance(worker_evidence, list):
        unknown_worker = non_worker_evidence_items(worker_evidence)
        if unknown_worker:
            fail(f"{label}.worker_evidence_required contains unsupported worker evidence keys: {unknown_worker!r}")
        lifecycle_in_worker = lifecycle_evidence_items(worker_evidence)
        if lifecycle_in_worker:
            fail(f"{label}.worker_evidence_required must only contain worker-owned evidence: {lifecycle_in_worker!r}")
    if isinstance(lifecycle_evidence, list):
        unknown_lifecycle = non_lifecycle_evidence_items(lifecycle_evidence)
        if unknown_lifecycle:
            fail(f"{label}.lifecycle_evidence_required contains unsupported lifecycle evidence keys: {unknown_lifecycle!r}")
        worker_in_lifecycle = worker_evidence_items(lifecycle_evidence)
        if worker_in_lifecycle:
            fail(f"{label}.lifecycle_evidence_required must only contain factoryd-owned lifecycle evidence: {worker_in_lifecycle!r}")
    if not isinstance(evidence_required, list) or not any(str(item).strip() for item in evidence_required):
        fail(f"{label}.evidence_required must be a non-empty list")
    known_evidence_keys = WORKER_EVIDENCE_KEYS | LIFECYCLE_EVIDENCE_KEYS
    unknown_evidence = [
        item for item in evidence_required
        if str(item).strip().lower().replace("-", "_") not in known_evidence_keys
    ]
    if unknown_evidence:
        fail(f"{label}.evidence_required contains unsupported evidence keys: {unknown_evidence!r}")
    require_worker_evidence_mirror(label, evidence_required, worker_evidence)
    lifecycle_defaults = lifecycle_evidence_items(evidence_required)
    require_lifecycle_evidence_mirror(f"{label}.lifecycle_evidence_required", lifecycle_defaults, lifecycle_evidence)

def validate_runner_ready_field_sync(execution_plan, validation_contract):
    embedded = (
        (execution_plan.get("validation_contract") or {})
        .get("factoryd_runtime_requirements") or {}
    ).get("runner_ready_fields")
    standalone = (
        (validation_contract.get("factoryd_runtime_requirements") or {})
        .get("runner_ready_fields")
    )
    if embedded != standalone:
        fail(
            "execution-plan.validation_contract.factoryd_runtime_requirements.runner_ready_fields "
            "must match validation-contract.factoryd_runtime_requirements.runner_ready_fields"
        )

def missing_grant_value(value):
    if value is None or value == []:
        return True
    if isinstance(value, str):
        return not value.strip()
    return False

def public_release_boundary_error(label, document):
    slices = document.get("delivery_slices") or []
    release_slices = [
        item for item in slices
        if isinstance(item, dict)
        and str(item.get("slice_id", "")).startswith("release-demo-and-product-signals")
    ]
    all_boundaries = [
        item for item in slices
        if isinstance(item, dict) and item.get("public_release_boundary") is True
    ]
    if not release_slices:
        if all_boundaries:
            boundary_ids = ", ".join(str(item.get("slice_id", "<missing>")) for item in all_boundaries)
            return f"{label} must not declare public release boundaries outside a release/demo/product-signal slice; found {boundary_ids}"
        return None
    if len(all_boundaries) != 1:
        boundary_ids = ", ".join(str(item.get("slice_id", "<missing>")) for item in all_boundaries)
        if not boundary_ids:
            boundary_ids = "<none>"
        return f"{label} must declare exactly one public release boundary across all delivery_slices; found {len(all_boundaries)}: {boundary_ids}"
    boundary = all_boundaries[0]
    boundary_id = str(boundary.get("slice_id", ""))
    if not boundary_id.startswith("release-demo-and-product-signals"):
        return f"{label} public release boundary must be on the final release/demo/product-signal slice, got {boundary_id or '<missing>'}"
    expected_boundary = release_slices[-1].get("slice_id")
    if boundary.get("slice_id") != expected_boundary:
        return f"{label} public release boundary must be the final release/demo/product-signal slice {expected_boundary}"
    if not boundary.get("required_for_completion"):
        return f"{label}.{expected_boundary}.required_for_completion must be true"
    return None


def validate_public_release_boundary(documents):
    for label, document in documents:
        error = public_release_boundary_error(label, document)
        if error:
            fail(error)


def validate_context_brief(context, provider_task_ids):
    decisions = context.get("alignment_decisions")
    if not isinstance(decisions, dict):
        fail("context-brief.json missing alignment_decisions")
    model_provider = decisions.get("model_provider_endpoint")
    if not provider_task_ids:
        return
    if not isinstance(model_provider, dict):
        fail("context-brief.json.alignment_decisions.model_provider_endpoint is required for provider-gated tasks")
    if model_provider.get("required_grant") != "model_provider_endpoint":
        fail("context-brief.json.alignment_decisions.model_provider_endpoint.required_grant must be model_provider_endpoint")
    if model_provider.get("generic_grants_sufficient") is not False:
        fail("context-brief.json.alignment_decisions.model_provider_endpoint.generic_grants_sufficient must be false")
    dispatch_refs = {
        str(task_id).strip()
        for task_id in model_provider.get("required_before_dispatch") or []
        if str(task_id).strip()
    }
    missing = sorted(provider_task_ids - dispatch_refs)
    if missing:
        fail("context-brief.json.alignment_decisions.model_provider_endpoint.required_before_dispatch missing provider-gated tasks: " + ", ".join(missing))


def validated_remaining_task_refs(item, task_ids, item_id):
    item_refs = item.get("task_refs") or []
    refs = item["remaining_task_refs"] if "remaining_task_refs" in item else []
    if not isinstance(refs, list):
        fail(f"scope closure item {item_id} remaining_task_refs must be a list")
    if not isinstance(item_refs, list):
        fail(f"scope closure item {item_id} task_refs must be a list")
    item_ref_ids = set(item_refs)
    normalized = set()
    for ref in refs:
        if not isinstance(ref, str) or ref.strip() != ref or not ref:
            fail(f"scope closure item {item_id} remaining_task_refs must contain canonical task ids")
        normalized.add(ref)
    if not normalized.issubset(task_ids):
        fail(f"scope closure item {item_id} remaining_task_refs references missing task")
    if not normalized.issubset(item_ref_ids):
        fail(f"scope closure item {item_id} remaining_task_refs must be a subset of task_refs")
    return normalized


def validate_model_provider_gate(task):
    task_id = task.get("task_id") or "T9"
    if task.get("requires_model_provider_endpoint") is not True:
        fail(f"{task_id}.requires_model_provider_endpoint must be true for provider-backed distill work")
    for key in ["requires_network", "requires_credentials"]:
        if task.get(key) is not True:
            fail(f"{task_id}.{key} must be true for provider-backed distill work")
    requirements = task.get("model_provider_requirements")
    if not isinstance(requirements, dict) or requirements.get("required_grant") != "model_provider_endpoint":
        fail(f"{task_id}.model_provider_requirements must require model_provider_endpoint")
    provider_surfaces = {str(value) for value in requirements.get("provider_surfaces") or []}
    missing_surfaces = {"openai_compatible_http", "anthropic_messages_http"} - provider_surfaces
    if missing_surfaces:
        fail(f"{task_id}.model_provider_requirements.provider_surfaces missing {sorted(missing_surfaces)}")
    required_fields = {str(value) for value in requirements.get("required_fields") or []}
    expected_required_fields = {
        "provider_identity",
        "provider_model",
        "provider_endpoint_or_base_url",
        "credential_environment",
        "budget_posture",
        "redaction_posture",
        "network_allowlist",
    }
    missing_required_fields = sorted(expected_required_fields - required_fields)
    if missing_required_fields:
        fail(f"{task_id}.model_provider_requirements.required_fields missing {missing_required_fields}")
    if task_id == "T9":
        for key in ["requires_human_approval", "requires_network", "requires_credentials"]:
            if task.get(key) is not True:
                fail(f"{task_id}.{key} must remain true because T9 mixes model-provider, network, credential, API, webhook, and hosted-service work")
    required_grant_fields = [
        "evidence_ref",
        "network_allowlist",
        "provider_identity",
        "provider_model",
        "credential_environment",
        "budget_posture",
        "redaction_posture",
    ]
    required_string_fields = [
        "evidence_ref",
        "provider_identity",
        "provider_model",
        "credential_environment",
        "budget_posture",
        "redaction_posture",
    ]

    def validate_provider_grant_fields(candidate, label):
        missing = [
            field for field in required_grant_fields
            if field not in candidate or missing_grant_value(candidate[field])
        ]
        if missing:
            fail(f"{label} missing fields: {missing}")
        non_string_fields = [
            field for field in required_string_fields
            if field in candidate and not isinstance(candidate.get(field), str)
        ]
        if non_string_fields:
            fail(f"{label} fields must be non-empty strings: {non_string_fields}")
        allowlist = candidate.get("network_allowlist")
        if not isinstance(allowlist, list) or not all(isinstance(item, str) and item.strip() for item in allowlist):
            fail(f"{label} network_allowlist must be a non-empty string list")
        provider_endpoint_value = candidate.get("provider_endpoint", "")
        base_url_value = candidate.get("base_url", "")
        if provider_endpoint_value not in (None, "") and not isinstance(provider_endpoint_value, str):
            fail(f"{label} provider_endpoint must be a string")
        if base_url_value not in (None, "") and not isinstance(base_url_value, str):
            fail(f"{label} base_url must be a string")
        provider_endpoint = provider_endpoint_value.strip() if isinstance(provider_endpoint_value, str) else ""
        base_url = base_url_value.strip() if isinstance(base_url_value, str) else ""
        provider_endpoint_or_base_url = provider_endpoint or base_url
        if not provider_endpoint_or_base_url:
            fail(f"{label} must include provider_endpoint or base_url")
        return provider_endpoint_or_base_url

    seed_grants = ((task.get("factoryd_runtime") or {}).get("capability_grants")) or []
    active_grants = factoryd_config_capability_grants()
    active_wildcard_grants = [
        grant for grant in active_grants
        if isinstance(grant, dict)
        and str(grant.get("task_id", "")).strip() == "*"
        and grant.get("capability") == "model_provider_endpoint"
    ]
    if active_wildcard_grants:
        fail(f"{task_id}.active model_provider_endpoint grants must be task-scoped, not wildcard")
    seed_matching = [
        grant for grant in seed_grants
        if isinstance(grant, dict)
        and str(grant.get("task_id", "")).strip() in {"*", task_id}
        and grant.get("capability") == "model_provider_endpoint"
    ]
    if any(grant.get("approved") is True for grant in seed_matching):
        fail(f"{task_id}.seed model_provider_endpoint grants must stay approved false; active approvals belong in .factory/factoryd.json")
    for seed_grant in seed_matching:
        validate_provider_grant_fields(seed_grant, f"{task_id}.seed model_provider_endpoint grant")
    active_matching = [
        grant for grant in active_grants
        if isinstance(grant, dict)
        and str(grant.get("task_id", "")).strip() == task_id
        and grant.get("capability") == "model_provider_endpoint"
    ]
    matching = [*seed_matching, *active_matching]
    if not matching:
        fail(f"{task_id} must include one seed wildcard or task-scoped model_provider_endpoint grant in factoryd_runtime.capability_grants, or one task-scoped active .factory/factoryd.json config grant")
    grant = next((candidate for candidate in matching if candidate.get("approved") is True), matching[0])
    approved = grant.get("approved")
    if approved not in (False, True):
        fail(f"{task_id}.model_provider_endpoint grant approved flag must be true or false")
    validate_provider_grant_fields(grant, f"{task_id}.model_provider_endpoint grant")
    if approved is True:
        checked_values = [
            grant.get("provider_identity"),
            grant.get("provider_model"),
            grant.get("provider_endpoint"),
            grant.get("base_url"),
            grant.get("credential_environment"),
            grant.get("budget_posture"),
            grant.get("redaction_posture"),
            *list(grant.get("network_allowlist") or []),
        ]
        if any("pending-approved" in str(value).lower() or str(value).lower().startswith("pending-") for value in checked_values):
            fail(f"{task_id}.approved model_provider_endpoint grant must not use pending placeholders")
    if "model_provider_endpoint" not in str(grant.get("evidence_ref")):
        fail(f"{task_id}.model_provider_endpoint grant evidence_ref must cite the alignment decision")
    joined_stop_conditions = "\n".join(str(value) for value in task.get("stop_conditions") or [])
    if "model_provider_endpoint grant" not in joined_stop_conditions:
        fail(f"{task_id}.stop_conditions must fail closed without model_provider_endpoint grant")


def model_provider_gate_task(task_id="T9", grant_task_id="*"):
    return {
        "task_id": task_id,
        "requires_model_provider_endpoint": True,
        "requires_network": True,
        "requires_credentials": True,
        "requires_human_approval": task_id == "T9",
        "model_provider_requirements": {
            "required_grant": "model_provider_endpoint",
            "provider_surfaces": ["openai_compatible_http", "anthropic_messages_http"],
            "required_fields": [
                "provider_identity",
                "provider_model",
                "provider_endpoint_or_base_url",
                "credential_environment",
                "budget_posture",
                "redaction_posture",
                "network_allowlist",
            ],
        },
        "factoryd_runtime": {
            "capability_grants": [
                {
                    "task_id": grant_task_id,
                    "capability": "model_provider_endpoint",
                    "approved": False,
                    "evidence_ref": ".factory/artifacts/approvals/model_provider_endpoint.md",
                    "network_allowlist": ["pending-approved-provider-host"],
                    "provider_identity": "pending-approved-provider",
                    "provider_model": "pending-approved-model",
                    "provider_endpoint": "pending-approved-provider-endpoint",
                    "credential_environment": "pending-approved-credential-environment",
                    "budget_posture": "pending-approved-budget",
                    "redaction_posture": "pending-approved-redaction",
                }
            ]
        },
        "stop_conditions": ["missing model_provider_endpoint grant"],
    }


def validate_runner_ready_task_fields(task, task_id):
    for key in RUNNER_READY_TASK_FIELDS:
        missing_values = (None, "") if key == "semantic_invariants" else (None, "", [])
        if key not in task or task[key] in missing_values:
            fail(f"{task_id} missing runner-ready field: {key}")
    semantic_invariants = task.get("semantic_invariants")
    if not isinstance(semantic_invariants, list) or not any(str(item).strip() for item in semantic_invariants):
        fail(f"{task_id}.semantic_invariants must be a non-empty list")
    normalized_semantic_invariants = []
    for index, invariant in enumerate(semantic_invariants):
        if not isinstance(invariant, str) or not invariant.strip():
            fail(f"{task_id}.semantic_invariants[{index}] must be a non-empty string")
        normalized_semantic_invariants.append(invariant.strip())
    if len(set(normalized_semantic_invariants)) != len(normalized_semantic_invariants):
        fail(f"{task_id}.semantic_invariants must not contain duplicate entries")
    worker_evidence = task.get("worker_evidence_required")
    if not isinstance(worker_evidence, list) or not any(str(item).strip() for item in worker_evidence):
        fail(f"{task_id}.worker_evidence_required must be a non-empty list")
    unknown_worker = non_worker_evidence_items(worker_evidence)
    if unknown_worker:
        fail(f"{task_id}.worker_evidence_required contains unsupported worker evidence keys: {unknown_worker!r}")
    lifecycle_evidence = task.get("lifecycle_evidence_required")
    if not isinstance(lifecycle_evidence, list) or not any(str(item).strip() for item in lifecycle_evidence):
        fail(f"{task_id}.lifecycle_evidence_required must be a non-empty list")
    unknown_lifecycle = non_lifecycle_evidence_items(lifecycle_evidence)
    if unknown_lifecycle:
        fail(f"{task_id}.lifecycle_evidence_required contains unsupported lifecycle evidence keys: {unknown_lifecycle!r}")
    worker_in_lifecycle = worker_evidence_items(lifecycle_evidence)
    if worker_in_lifecycle:
        fail(
            f"{task_id}.lifecycle_evidence_required must only contain factoryd-owned lifecycle evidence; "
            f"move worker evidence to worker_evidence_required: {worker_in_lifecycle!r}"
        )
    evidence_required = task.get("evidence_required")
    if not isinstance(evidence_required, list) or not any(str(item).strip() for item in evidence_required):
        fail(f"{task_id}.evidence_required must be a non-empty list")
    unknown_evidence = non_worker_evidence_items(evidence_required)
    if unknown_evidence:
        fail(f"{task_id}.evidence_required contains unsupported worker evidence keys: {unknown_evidence!r}")
    lifecycle_in_worker = lifecycle_evidence_items(evidence_required)
    if lifecycle_in_worker:
        fail(
            f"{task_id}.evidence_required must only contain worker-owned evidence; "
            f"move lifecycle evidence to lifecycle_evidence_required: {lifecycle_in_worker!r}"
        )
    require_worker_evidence_mirror(task_id, evidence_required, worker_evidence)
    lifecycle_in_worker_subset = lifecycle_evidence_items(worker_evidence)
    if lifecycle_in_worker_subset:
        fail(
            f"{task_id}.worker_evidence_required must only contain worker-owned evidence; "
            f"move lifecycle evidence to lifecycle_evidence_required: {lifecycle_in_worker_subset!r}"
        )
    inheritance = task.get("validation_contract_inheritance")
    if isinstance(inheritance, dict):
        inherited_evidence = inheritance.get("evidence_required")
        if "evidence_required" in inheritance and not isinstance(inherited_evidence, list):
            fail(f"{task_id}.validation_contract_inheritance.evidence_required must be a list")
        if isinstance(inherited_evidence, list):
            unknown_inherited_evidence = non_worker_evidence_items(inherited_evidence)
            if unknown_inherited_evidence:
                fail(
                    f"{task_id}.validation_contract_inheritance.evidence_required "
                    f"contains unsupported worker evidence keys: {unknown_inherited_evidence!r}"
                )
            inherited_lifecycle_in_worker = lifecycle_evidence_items(inherited_evidence)
            if inherited_lifecycle_in_worker:
                fail(
                    f"{task_id}.validation_contract_inheritance.evidence_required must only contain "
                    f"worker-owned evidence; move lifecycle evidence to lifecycle_evidence_required: "
                    f"{inherited_lifecycle_in_worker!r}"
                )
            require_worker_evidence_mirror(
                f"{task_id}.validation_contract_inheritance",
                inherited_evidence,
                inheritance.get("worker_evidence_required"),
            )
        inherited_worker = inheritance.get("worker_evidence_required")
        if "worker_evidence_required" in inheritance and not isinstance(inherited_worker, list):
            fail(f"{task_id}.validation_contract_inheritance.worker_evidence_required must be a list")
        if isinstance(inherited_worker, list):
            unknown_inherited_worker = non_worker_evidence_items(inherited_worker)
            if unknown_inherited_worker:
                fail(
                    f"{task_id}.validation_contract_inheritance.worker_evidence_required "
                    f"contains unsupported worker evidence keys: {unknown_inherited_worker!r}"
                )
            inherited_lifecycle_in_worker_subset = lifecycle_evidence_items(inherited_worker)
            if inherited_lifecycle_in_worker_subset:
                fail(
                    f"{task_id}.validation_contract_inheritance.worker_evidence_required must only contain "
                    f"worker-owned evidence; move lifecycle evidence to lifecycle_evidence_required: "
                    f"{inherited_lifecycle_in_worker_subset!r}"
                )
            require_worker_values_present(
                f"{task_id}.worker_evidence_required",
                inherited_worker,
                worker_evidence,
            )
            require_worker_values_present(
                f"{task_id}.evidence_required",
                inherited_worker,
                evidence_required,
            )
        inherited_lifecycle = inheritance.get("lifecycle_evidence_required")
        if "lifecycle_evidence_required" in inheritance and not isinstance(inherited_lifecycle, list):
            fail(f"{task_id}.validation_contract_inheritance.lifecycle_evidence_required must be a list")
        if isinstance(inherited_lifecycle, list):
            unknown_inherited_lifecycle = non_lifecycle_evidence_items(inherited_lifecycle)
            if unknown_inherited_lifecycle:
                fail(
                    f"{task_id}.validation_contract_inheritance.lifecycle_evidence_required "
                    f"contains unsupported lifecycle evidence keys: {unknown_inherited_lifecycle!r}"
                )
            inherited_worker_in_lifecycle = worker_evidence_items(inherited_lifecycle)
            if inherited_worker_in_lifecycle:
                fail(
                    f"{task_id}.validation_contract_inheritance.lifecycle_evidence_required must only contain "
                    f"factoryd-owned lifecycle evidence; move worker evidence to worker_evidence_required: "
                    f"{inherited_worker_in_lifecycle!r}"
                )
            require_lifecycle_evidence_mirror(
                f"{task_id}.lifecycle_evidence_required",
                inherited_lifecycle,
                lifecycle_evidence,
            )
    required_scorecard_key = "proof_of_behavior_scorecard"
    proof_level = str(task.get("required_proof_level", "")).strip()
    scorecard_required = task.get("proof_scorecard_required") is True or proof_level in {
        "workflow_behavior",
        "user_visible_behavior",
    }
    if scorecard_required:
        normalized_worker = {
            str(item).strip().lower().replace("-", "_")
            for item in worker_evidence
        }
        normalized_evidence = {
            str(item).strip().lower().replace("-", "_")
            for item in evidence_required or []
        }
        if required_scorecard_key not in normalized_worker or required_scorecard_key not in normalized_evidence:
            fail(
                f"{task_id} requires a proof-of-behavior scorecard but does not list "
                "proof-of-behavior-scorecard in worker evidence"
            )
        if isinstance(inheritance, dict):
            inherited_worker = inheritance.get("worker_evidence_required")
            inherited_evidence = inheritance.get("evidence_required")
            normalized_inherited_worker = {
                str(item).strip().lower().replace("-", "_")
                for item in inherited_worker or []
            }
            normalized_inherited_evidence = {
                str(item).strip().lower().replace("-", "_")
                for item in inherited_evidence or []
            }
            if (
                isinstance(inherited_worker, list)
                and required_scorecard_key not in normalized_inherited_worker
            ) or (
                isinstance(inherited_evidence, list)
                and required_scorecard_key not in normalized_inherited_evidence
            ):
                fail(
                    f"{task_id}.validation_contract_inheritance must preserve "
                    "proof-of-behavior-scorecard worker evidence"
                )
    for index, requirement in enumerate(task.get("acceptance_result_requirements") or []):
        if not isinstance(requirement, dict):
            fail(f"{task_id}.acceptance_result_requirements[{index}] must be an object")
        requirement_evidence = requirement.get("evidence_required")
        if not isinstance(requirement_evidence, list) or not any(str(item).strip() for item in requirement_evidence):
            fail(f"{task_id}.acceptance_result_requirements[{index}].evidence_required must be a non-empty list")
        unknown_requirement_evidence = non_worker_evidence_items(requirement_evidence)
        if unknown_requirement_evidence:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].evidence_required "
                f"contains unsupported worker evidence keys: {unknown_requirement_evidence!r}"
            )
        nested_lifecycle_in_worker = lifecycle_evidence_items(requirement_evidence)
        if nested_lifecycle_in_worker:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].evidence_required must only contain "
                f"worker-owned evidence; move lifecycle evidence to lifecycle_evidence_required: "
                f"{nested_lifecycle_in_worker!r}"
            )
        nested_worker = requirement.get("worker_evidence_required")
        if not isinstance(nested_worker, list) or not any(str(item).strip() for item in nested_worker):
            fail(f"{task_id}.acceptance_result_requirements[{index}].worker_evidence_required must be a non-empty list")
        unknown_nested_worker = non_worker_evidence_items(nested_worker)
        if unknown_nested_worker:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].worker_evidence_required "
                f"contains unsupported worker evidence keys: {unknown_nested_worker!r}"
            )
        nested_lifecycle = requirement.get("lifecycle_evidence_required")
        if not isinstance(nested_lifecycle, list) or not any(str(item).strip() for item in nested_lifecycle):
            fail(f"{task_id}.acceptance_result_requirements[{index}].lifecycle_evidence_required must be a non-empty list")
        unknown_nested_lifecycle = non_lifecycle_evidence_items(nested_lifecycle)
        if unknown_nested_lifecycle:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].lifecycle_evidence_required "
                f"contains unsupported lifecycle evidence keys: {unknown_nested_lifecycle!r}"
            )
        nested_worker_in_lifecycle = worker_evidence_items(nested_lifecycle)
        if nested_worker_in_lifecycle:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].lifecycle_evidence_required must only contain "
                f"factoryd-owned lifecycle evidence: {nested_worker_in_lifecycle!r}"
            )
        nested_lifecycle_in_worker_subset = lifecycle_evidence_items(nested_worker)
        if nested_lifecycle_in_worker_subset:
            fail(
                f"{task_id}.acceptance_result_requirements[{index}].worker_evidence_required must only contain "
                f"worker-owned evidence: {nested_lifecycle_in_worker_subset!r}"
            )
        require_worker_evidence_mirror(
            f"{task_id}.acceptance_result_requirements[{index}]",
            requirement_evidence,
            nested_worker,
        )
        if scorecard_required:
            normalized_nested_worker = {
                str(item).strip().lower().replace("-", "_")
                for item in nested_worker
            }
            normalized_nested_evidence = {
                str(item).strip().lower().replace("-", "_")
                for item in requirement_evidence
            }
            if (
                required_scorecard_key not in normalized_nested_worker
                or required_scorecard_key not in normalized_nested_evidence
            ):
                fail(
                    f"{task_id}.acceptance_result_requirements[{index}] requires "
                    "proof-of-behavior scorecard worker evidence"
                )
    if task.get("required_proof_level") not in REQUIRED_PROOF_LEVELS:
        fail(f"{task_id}.required_proof_level must be one of {sorted(REQUIRED_PROOF_LEVELS)}")
    if not isinstance(task.get("artifact_budget_refs"), list):
        fail(f"{task_id}.artifact_budget_refs must be a non-empty list")
    redaction = task.get("redaction_posture")
    if not isinstance(redaction, dict):
        fail(f"{task_id}.redaction_posture must be an object")
    if redaction.get("classification") not in {"internal", "customer_safe", "public"}:
        fail(f"{task_id}.redaction_posture.classification is invalid")
    if not isinstance(redaction.get("customer_safe"), bool):
        fail(f"{task_id}.redaction_posture.customer_safe must be boolean")


def validate_task_required_review(task, task_id, required):
    review = task.get("required_review")
    if not required and review is None:
        return
    if not isinstance(review, dict):
        fail(f"{task_id}.required_review must be an object")
    if review.get("required") is not True:
        fail(f"{task_id}.required_review.required must be true for remaining implementation work")
    if review.get("review_type") not in {"code", "architecture", "security"}:
        fail(f"{task_id}.required_review.review_type must be code, architecture, or security")
    if review.get("reviewer_class") != "peer_agent":
        fail(f"{task_id}.required_review.reviewer_class must be peer_agent")


def main():
    args = parse_args(sys.argv[1:])
    if args["self_test"]:
        from repo_pack_self_test import self_test

        self_test()
        return
    root = ROOT
    for rel in REQUIRED:
        if not (root / rel).exists():
            fail(f"missing required repo-pack file: {rel}")
    cfg = load_json_file(args["factoryd_config"])
    autoship_cfg = json.loads((root / ".factory/factoryd.autoship.example.json").read_text())
    repos = cfg.get("repos") or {}
    if len(repos) != 1:
        fail("factoryd config must define exactly one repo")
    repo_key = next(iter(repos.keys()))
    repo = repos[repo_key]
    autoship_repos = autoship_cfg.get("repos") or {}
    autoship_repo = autoship_repos.get(repo_key)
    if not autoship_repo:
        fail("autoship config must define the same repo key as safe config")
    shipping = autoship_repo.get("shipping") or {}
    if autoship_repo.get("auto_ship") is not True or shipping.get("enabled") is not True:
        fail("autoship config must explicitly enable auto_ship and shipping.enabled")
    if shipping.get("provider") != "github_cli":
        fail("autoship config must use the github_cli provider")
    for key in ["push_required", "pr_required", "ci_required", "codex_review_required", "merge_required", "post_merge_required", "scope_closure_required"]:
        if shipping.get(key) is not True:
            fail(f"autoship config must require {key}")
    profile_path = (cfg.get("factory") or {}).get("profile_path")
    if profile_path != ".factory/profile.yaml":
        fail("factory.profile_path must point at generated .factory/profile.yaml")
    profile_text = (root / ".factory/profile.yaml").read_text()
    profile_visibility = profile_visibility_from_text(profile_text)
    if profile_visibility != "public":
        fail("profile repo_visibility must be public")
    required_checks = json.loads((root / ".github/required-checks.json").read_text()).get("required_checks") or []
    if "validate" not in required_checks:
        fail("required-checks.json must require validate")
    makefile_text = (root / "Makefile").read_text()
    dev_guide_text = (root / "docs/dev/dev_guides.md").read_text()
    for needle in ["test-coverage:", "check_go_coverage.py", "prepush-full: lint-fast test-fast test-coverage test-contracts"]:
        if needle not in makefile_text:
            fail(f"Makefile missing coverage gate token {needle!r}")
    for needle in ["Coverage Gates", "make test-coverage", ">= 75%"]:
        if needle not in dev_guide_text:
            fail(f"docs/dev/dev_guides.md missing coverage gate token {needle!r}")
    validate_text = (root / ".github/workflows/validate.yml").read_text()
    codeql_text = (root / ".github/workflows/codeql.yml").read_text()
    for needle in ["permissions:", "concurrency:", "timeout-minutes:", "actions/checkout@v6.0.2", "actions/setup-go@v6.3.0", "go-version-file: go.mod", "make prepush-full"]:
        if needle not in validate_text:
            fail(f"validate workflow missing {needle}")
    for needle in ["permissions:", "concurrency:", "timeout-minutes:", "actions/checkout@v6.0.2", "actions/setup-go@v6.3.0", "github/codeql-action/init@v4", "github/codeql-action/analyze@v4"]:
        if needle not in codeql_text:
            fail(f"CodeQL workflow missing {needle}")
    if profile_visibility == "public":
        if "CodeQL analyze" not in required_checks:
            fail("public generated repos must require CodeQL analyze")
        if "vars.CODEQL_ENABLED" in codeql_text:
            fail("public generated repos must not gate CodeQL behind CODEQL_ENABLED")
    else:
        if "vars.CODEQL_ENABLED == 'true'" not in codeql_text:
            fail("private generated repos must keep CodeQL opt-in behind CODEQL_ENABLED")
    for key in ["acceptance_ledger", "task_packets", "scope_closure_map", "validation_contract", "validation_commands", "worker_type"]:
        if not repo.get(key):
            fail(f"factoryd config missing {key}")
    if "capability_grants" not in repo or not isinstance(repo["capability_grants"], list):
        fail("factoryd config must declare capability_grants as a list")
    validate_architecture_budget_policy(repo, "factoryd config")
    validate_architecture_budget_policy(autoship_repo, "autoship config")
    active_repo = active_factoryd_repo_config(repo_key)
    validate_active_architecture_budget_policy(active_repo)
    validate_active_capability_grants(active_repo)
    for rel in [repo["acceptance_ledger"], repo["task_packets"], repo["scope_closure_map"], repo["validation_contract"]]:
        if not (root / rel).exists():
            fail(f"factoryd config references missing file: {rel}")
    packets = json.loads((root / repo["task_packets"]).read_text())
    if packets.get("artifact_type") != "task_packets":
        fail("task-packets.json artifact_type must be task_packets")
    plan_dir = (root / repo["task_packets"]).parent
    plan_dir_ref = str(Path(repo["task_packets"]).parent).replace("\\", "/")
    execution_plan_path = plan_dir / "execution-plan.json"
    if not execution_plan_path.exists():
        fail("execution-plan.json is required next to task-packets.json")
    context_brief_path = plan_dir / "context-brief.json"
    if not context_brief_path.exists():
        fail("context-brief.json is required next to task-packets.json")
    execution_plan = json.loads(execution_plan_path.read_text())
    context_brief = json.loads(context_brief_path.read_text())
    validation_contract = json.loads((root / repo["validation_contract"]).read_text())
    task_supervision_policy = execution_plan.get("task_supervision_policy") or {}
    if task_supervision_policy.get("skill_ref") != "factory://skills/task-supervisor":
        fail("execution-plan task_supervision_policy.skill_ref must be factory://skills/task-supervisor")
    if task_supervision_policy.get("evidence_artifact_type") != "task_supervisor_report":
        fail("execution-plan task_supervision_policy.evidence_artifact_type must be task_supervisor_report")
    if task_supervision_policy.get("evidence_path") != ".factory/artifacts/task-supervisor-runs/<mission>/<timestamp>.json":
        fail("execution-plan task_supervision_policy.evidence_path must point at task-supervisor-runs")
    validate_validation_contract_evidence_split(validation_contract, "validation-contract")
    validate_validation_contract_evidence_split(execution_plan.get("validation_contract"), "execution-plan.validation_contract")
    validate_runner_ready_field_sync(execution_plan, validation_contract)
    ledger = json.loads((root / repo["acceptance_ledger"]).read_text())
    if ledger.get("artifact_type") != "acceptance_ledger":
        fail("acceptance ledger must use artifact_type acceptance_ledger")
    ledger_items = ledger.get("items") or []
    if not ledger_items:
        fail("acceptance ledger must contain at least one item")
    if ledger.get("acceptance_item_count") != len(ledger_items):
        fail("acceptance ledger acceptance_item_count must match items length")
    ledger_ids = {item.get("acceptance_item_id") for item in ledger_items}
    if len(ledger_ids) != len(ledger_items):
        fail("acceptance ledger item ids must be unique")
    validation_ref_re = re.compile(r"/task-packets[.]json#/tasks/([0-9]+)/validation_commands$")
    coverage = execution_plan.get("acceptance_ledger_coverage") or {}
    expected_refs = {
        "ledger_ref": repo["acceptance_ledger"],
        "acceptance_mapping_ref": plan_dir_ref + "/acceptance-mapping.json",
        "scope_closure_map_ref": repo["scope_closure_map"],
    }
    for key, expected in expected_refs.items():
        if coverage.get(key) != expected:
            fail(f"execution-plan acceptance_ledger_coverage.{key} must cite {expected}")
    if coverage.get("coverage_unit") != "acceptance_item":
        fail("execution-plan acceptance_ledger_coverage.coverage_unit must be acceptance_item")
    if coverage.get("group_only_refs_allowed") is not False:
        fail("execution-plan acceptance_ledger_coverage.group_only_refs_allowed must be false")
    if coverage.get("required_item_count") != len(ledger_items):
        fail("execution-plan acceptance_ledger_coverage.required_item_count must match acceptance ledger")
    mapping_path = plan_dir / "acceptance-mapping.json"
    if not mapping_path.exists():
        fail("acceptance-mapping.json is required next to task-packets.json")
    mapping = json.loads(mapping_path.read_text())
    groups = mapping.get("groups") or []
    tasks = packets.get("tasks") or []
    if not tasks:
        fail("task-packets.json must contain at least one task")
    task_id_values = [task.get("task_id") for task in tasks]
    if None in task_id_values:
        fail("task-packets.json contains a task without task_id")
    duplicate_task_ids = duplicate_values(task_id_values)
    if duplicate_task_ids:
        fail("task-packets task_id values must be unique: " + ", ".join(str(task_id) for task_id in duplicate_task_ids))
    task_ids = set(task_id_values)
    for item in ledger_items:
        for ref in item.get("validation_refs") or []:
            if "/tasks/T" in ref:
                fail(f"acceptance item {item.get('acceptance_item_id')} validation_refs must use task array indexes, not task ids")
            match = validation_ref_re.search(ref)
            if not match:
                fail(f"acceptance item {item.get('acceptance_item_id')} validation_refs must point at task-packets.json#/tasks/<index>/validation_commands")
            if int(match.group(1)) >= len(tasks):
                fail(f"acceptance item {item.get('acceptance_item_id')} validation_refs points past task array")
    for group in groups:
        refs = set(group.get("task_refs") or [])
        if not refs or not refs.issubset(task_ids):
            fail(f"acceptance group {group.get('group_id')} references missing task")
        item_ids = set(group.get("acceptance_item_ids") or [])
        if not item_ids:
            fail(f"acceptance group {group.get('group_id')} must reference acceptance_item_ids")
        if not item_ids.issubset(ledger_ids):
            fail(f"acceptance group {group.get('group_id')} references unknown acceptance item ids")
        item_mappings = group.get("item_mappings") or []
        mapped_item_ids = {item.get("acceptance_item_id") for item in item_mappings}
        if mapped_item_ids != item_ids:
            fail(f"acceptance group {group.get('group_id')} item_mappings must cover the group's acceptance items")
        for item in item_mappings:
            item_refs = set(item.get("task_refs") or [])
            if not item_refs or not item_refs.issubset(task_ids):
                fail(f"acceptance item {item.get('acceptance_item_id')} references missing task")
            if not item.get("delivery_slice_refs"):
                fail(f"acceptance item {item.get('acceptance_item_id')} missing delivery_slice_refs")
    closure = json.loads((root / repo["scope_closure_map"]).read_text())
    closure_items = closure.get("items") or []
    if len(closure_items) != len(ledger_items):
        fail("scope-closure map must contain one item per acceptance ledger item")
    closure_ids = {item.get("scope_item_id") for item in closure_items}
    if closure_ids != ledger_ids:
        fail("scope-closure item ids must exactly match acceptance ledger item ids")
    for item in closure_items:
        item_id = item.get("scope_item_id")
        if item.get("acceptance_item_ids") != [item_id]:
            fail(f"scope closure item {item_id} must close exactly one acceptance item")
        refs = set(item.get("task_refs") or [])
        if not refs or not refs.issubset(task_ids):
            fail(f"scope closure item {item_id} references missing task")
        if not item.get("delivery_slice_refs"):
            fail(f"scope closure item {item_id} missing delivery_slice_refs")
    declared_slices = {item.get("slice_id") for item in execution_plan.get("delivery_slices") or []}
    if declared_slices:
        packet_slices = {item.get("slice_id") for item in packets.get("delivery_slices") or []}
        if packet_slices != declared_slices:
            fail("task-packets delivery_slices must match execution-plan delivery_slices")
    provider_task_ids = {
        task.get("task_id")
        for task in tasks
        if task.get("task_id")
        and (
            task.get("requires_model_provider_endpoint") is True
            or PROVIDER_ACCEPTANCE_IDS & set(task.get("acceptance_item_ids") or [])
        )
    }
    validate_context_brief(context_brief, provider_task_ids)
    validate_public_release_boundary([
        ("execution-plan", execution_plan),
        ("task-packets", packets),
        ("validation-contract", validation_contract),
        ("acceptance-mapping", mapping),
        ("scope-closure-map", closure),
    ])
    remaining_task_ids = {
        task_id
        for item in closure_items
        for task_id in validated_remaining_task_refs(item, task_ids, item.get("scope_item_id"))
    }
    for index, task in enumerate(tasks):
        task_id = task.get("task_id") or f"task[{index}]"
        validate_runner_ready_task_fields(task, task_id)
        validate_task_required_review(task, task_id, task_id in remaining_task_ids)
        lifecycle = task.get("lifecycle_gates") or {}
        if "ship_pr_required" in lifecycle:
            fail(f"{task_id}.lifecycle_gates uses deprecated ship_pr_required")
        if lifecycle.get("commit_push_required") is not True:
            fail(f"{task_id}.lifecycle_gates.commit_push_required must be true")
        task_item_ids = set(task.get("acceptance_item_ids") or [])
        if len(task_item_ids) > 15:
            fail(f"{task_id}.acceptance_item_ids has {len(task_item_ids)} items; split runner-ready tasks at 15 or fewer acceptance items")
        if not task_item_ids.issubset(ledger_ids):
            fail(f"{task_id} references unknown acceptance item ids")
        result_ids = {item.get("acceptance_item_id") for item in task.get("acceptance_result_requirements") or []}
        if result_ids != task_item_ids:
            fail(f"{task_id}.acceptance_result_requirements must cover task acceptance_item_ids")
        if declared_slices:
            task_slices = set(task.get("delivery_slice_refs") or [])
            if not task_slices:
                fail(f"{task_id} missing delivery_slice_refs")
            if not task_slices.issubset(declared_slices):
                fail(f"{task_id} references unknown delivery_slice_refs")
        allowed_paths = set(task.get("allowed_paths") or [])
        normalized_allowed_paths = {normalize_repo_path(path) for path in allowed_paths}
        control_allowed = sorted(
            path for path in normalized_allowed_paths
            if path == ".factory/artifacts"
            or path == ".factory/artifacts/"
            or path == plan_dir_ref
            or path.startswith(plan_dir_ref + "/")
        )
        if control_allowed:
            fail(f"{task_id}.allowed_paths includes runtime-owned control artifact: {control_allowed}")
        validate_architecture_target_paths(task, task_id)
        validate_lifecycle_path_ownership(task, task_id)
        if task.get("worker_type") != "codex_cli":
            fail(f"{task_id}.worker_type must be codex_cli")
        runtime = task.get("factoryd_runtime")
        if not isinstance(runtime, dict) or "capability_grants" not in runtime or not isinstance(runtime["capability_grants"], list):
            fail(f"{task_id}.factoryd_runtime.capability_grants must be a list")
        if task.get("requires_model_provider_endpoint") is True or PROVIDER_ACCEPTANCE_IDS & task_item_ids:
            validate_model_provider_gate(task)
        scanner = task.get("security_scanner_gates") or {}
        if profile_visibility == "public":
            if scanner.get("required") is not True or scanner.get("status_check") != "CodeQL analyze":
                fail(f"{task_id}.security_scanner_gates must require CodeQL analyze for public repos")
    print("ok: repo pack")

if __name__ == "__main__":
    main()
