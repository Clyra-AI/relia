#!/usr/bin/env python3
from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from tempfile import TemporaryDirectory

import validate_repo_pack as validator
from repo_pack_architecture import (
    architecture_budget_unexcepted_failures,
    architecture_debt_exception_budget_type_error,
    architecture_debt_exception_expiry_error,
    validate_architecture_debt_exception_expiry,
)

FACTORYD_REPO_KEY = validator.FACTORYD_REPO_KEY
PROVIDER_ACCEPTANCE_IDS = validator.PROVIDER_ACCEPTANCE_IDS
ROOT = validator.ROOT
RUNNER_READY_TASK_FIELDS = validator.RUNNER_READY_TASK_FIELDS
active_factoryd_repo_config = validator.active_factoryd_repo_config
duplicate_values = validator.duplicate_values
factoryd_config_capability_grants = validator.factoryd_config_capability_grants
fail = validator.fail
model_provider_gate_task = validator.model_provider_gate_task
public_release_boundary_error = validator.public_release_boundary_error
validate_active_architecture_budget_policy = validator.validate_active_architecture_budget_policy
validate_active_capability_grants = validator.validate_active_capability_grants
validate_context_brief = validator.validate_context_brief
validate_architecture_target_paths = validator.validate_architecture_target_paths
validate_lifecycle_path_ownership = validator.validate_lifecycle_path_ownership
validate_model_provider_gate = validator.validate_model_provider_gate
validate_runner_ready_task_fields = validator.validate_runner_ready_task_fields
validate_task_required_review = validator.validate_task_required_review
validate_runner_ready_field_sync = validator.validate_runner_ready_field_sync
validate_validation_contract_evidence_split = validator.validate_validation_contract_evidence_split
validated_remaining_task_refs = validator.validated_remaining_task_refs

def self_test_public_release_boundary():
    valid = {
        "delivery_slices": [
            {"slice_id": "foundation", "public_release_boundary": False},
            {"slice_id": "release-demo-and-product-signals-part-1", "public_release_boundary": False},
            {
                "slice_id": "release-demo-and-product-signals-part-2",
                "public_release_boundary": True,
                "required_for_completion": True,
            },
        ]
    }
    error = public_release_boundary_error("self-test-valid", valid)
    if error:
        fail(f"valid public release boundary fixture failed: {error}")

    duplicate_boundary = {
        "delivery_slices": [
            {"slice_id": "foundation", "public_release_boundary": True},
            {"slice_id": "release-demo-and-product-signals-part-1", "public_release_boundary": False},
            {
                "slice_id": "release-demo-and-product-signals-part-2",
                "public_release_boundary": True,
                "required_for_completion": True,
            },
        ]
    }
    error = public_release_boundary_error("self-test-duplicate", duplicate_boundary)
    if not error or "exactly one public release boundary across all delivery_slices" not in error:
        fail("duplicate public release boundary fixture did not fail closed")

    misplaced_boundary = {
        "delivery_slices": [
            {"slice_id": "foundation", "public_release_boundary": False},
            {
                "slice_id": "release-demo-and-product-signals-part-1",
                "public_release_boundary": True,
                "required_for_completion": True,
            },
            {
                "slice_id": "release-demo-and-product-signals-part-2",
                "public_release_boundary": False,
                "required_for_completion": True,
            },
        ]
    }
    error = public_release_boundary_error("self-test-misplaced", misplaced_boundary)
    if not error or "final release/demo/product-signal slice" not in error:
        fail("misplaced public release boundary fixture did not fail closed")

def self_test():
    if ROOT.name == "scripts":
        fail("repo root resolution selected scripts/ instead of repository root")
    if not (ROOT / "scripts" / "validate_repo_pack.py").exists():
        fail("repo root resolution must be relative to this validator file")
    parsed = validator.parse_args(["--factoryd-config", ".factory/factoryd.behavior-version-control.json"])
    if parsed["factoryd_config"] != ROOT / ".factory" / "factoryd.behavior-version-control.json":
        fail("--factoryd-config did not resolve to the requested repo-local config")
    try:
        validator.parse_args(["--factoryd-config", "../outside.json"])
    except SystemExit:
        pass
    else:
        fail("--factoryd-config accepted an escaped repo path")
    try:
        validator.parse_args(["--unknown"])
    except SystemExit:
        pass
    else:
        fail("unknown validator argument did not fail closed")
    with TemporaryDirectory() as temp_dir:
        temp_root = Path(temp_dir)
        oversized = temp_root / "cmd" / "demo" / "main.go"
        oversized.parent.mkdir(parents=True)
        oversized.write_text("line\n" * 2501)
        sample_budget = {
            "source_extensions": [".go"],
            "excluded_dirs": [".git", ".factoryd", ".factory/tmp", ".relia", "workspaces"],
            "fail_line_threshold": 2500,
        }
        failures = architecture_budget_unexcepted_failures(temp_root, sample_budget, set(), {})
        if not failures or "cmd/demo/main.go" not in failures[0]:
            fail("architecture budget self-test expected unexcepted oversized source to fail")
        scratch = temp_root / ".factory" / "tmp" / "scratch.go"
        scratch.parent.mkdir(parents=True)
        scratch.write_text("line\n" * 2501)
        if any(".factory/tmp/scratch.go" in failure for failure in architecture_budget_unexcepted_failures(temp_root, sample_budget, set(), {})):
            fail("architecture budget self-test expected .factory/tmp scratch to be excluded")
        generated = temp_root / ".relia" / "models" / "generated.go"
        generated.parent.mkdir(parents=True)
        generated.write_text("line\n" * 2501)
        if any(".relia/models/generated.go" in failure for failure in architecture_budget_unexcepted_failures(temp_root, sample_budget, set(), {})):
            fail("architecture budget self-test expected .relia generated state to be excluded")
        if architecture_budget_unexcepted_failures(temp_root, sample_budget, {"cmd/demo/main.go"}, {"cmd/demo/main.go": 2501}):
            fail("architecture budget self-test expected exception-scoped source to pass")
        ceiling_failures = architecture_budget_unexcepted_failures(
            temp_root,
            sample_budget,
            {"cmd/demo/main.go"},
            {"cmd/demo/main.go": 2500},
        )
        if not ceiling_failures or "approved ceiling" not in ceiling_failures[0]:
            fail("architecture budget self-test expected exception growth over ceiling to fail")
        prefix_root = temp_root / "prefix-check"
        first_party_build = prefix_root / "internal" / "build" / "big.go"
        first_party_build.parent.mkdir(parents=True)
        first_party_build.write_text("line\n" * 2501)
        generated_build = prefix_root / "build" / "generated.go"
        generated_build.parent.mkdir(parents=True)
        generated_build.write_text("line\n" * 2501)
        nested_dependency = prefix_root / "packages" / "web" / "node_modules" / "dep" / "generated.go"
        nested_dependency.parent.mkdir(parents=True)
        nested_dependency.write_text("line\n" * 2501)
        prefix_budget = {**sample_budget, "excluded_dirs": [*sample_budget["excluded_dirs"], "build", "node_modules"]}
        prefix_failures = architecture_budget_unexcepted_failures(prefix_root, prefix_budget, set(), {})
        if not any("internal/build/big.go" in failure for failure in prefix_failures):
            fail("architecture budget self-test expected first-party internal/build source to fail")
        if any("build/generated.go" in failure for failure in prefix_failures):
            fail("architecture budget self-test expected root build output to be excluded")
        if any("node_modules/dep/generated.go" in failure for failure in prefix_failures):
            fail("architecture budget self-test expected nested node_modules dependency to be excluded")
    validate_architecture_debt_exception_expiry(
        "self-test-valid",
        {"expires_at": "2099-01-01T00:00:00Z"},
        datetime(2026, 7, 1, tzinfo=timezone.utc),
    )
    if not architecture_debt_exception_expiry_error(
        "self-test-expired",
        {"expires_at": "2026-06-30T00:00:00Z"},
        datetime(2026, 7, 1, tzinfo=timezone.utc),
    ):
        fail("architecture debt exception self-test expected expired evidence to fail")
    if not architecture_debt_exception_budget_type_error("self-test", {}):
        fail("architecture debt exception self-test expected missing budget_type to fail")
    if architecture_debt_exception_budget_type_error("self-test", {"budget_type": "source_file_lines"}):
        fail("architecture debt exception self-test expected source_file_lines budget_type to pass")
    if duplicate_values(["T1", "T2", "T1", "T2", "T3"]) != ["T1", "T2"]:
        fail("duplicate_values must preserve duplicate ids in first duplicate order")
    if "FR23-PROVIDER-ADAPTERS-AND-NO-LLM-MODE-001" not in PROVIDER_ACCEPTANCE_IDS:
        fail("provider gate fallback must include FR23 provider adapter acceptance item")
    if "MVP-IN-SCOPE-010" not in PROVIDER_ACCEPTANCE_IDS:
        fail("provider gate fallback must include tiered provider-embedding acceptance item")
    self_test_public_release_boundary()
    validate_model_provider_gate(model_provider_gate_task())
    sample_task = {key: "value" for key in RUNNER_READY_TASK_FIELDS}
    for key in [
        "allowed_paths",
        "semantic_invariants",
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
        "required_worker_chain",
        "test_matrix_refs",
        "ci_lane_refs",
        "ci_control_refs",
        "coverage_policy_refs",
        "security_scanner_gates",
        "engineering_policy_refs",
        "architecture_guidance_refs",
        "docs_sync_refs",
        "acceptance_item_ids",
        "artifact_budget_refs",
    ]:
        sample_task[key] = ["value"]
    sample_task["evidence_required"] = ["validation_report", "proof-of-behavior-scorecard"]
    sample_task["worker_evidence_required"] = ["validation_report", "proof-of-behavior-scorecard"]
    sample_task["lifecycle_evidence_required"] = ["scope_closure_report"]
    sample_task["factoryd_runtime"] = {"worker_type": "codex_cli"}
    sample_task["lifecycle_gates"] = {"commit_push_required": True}
    sample_task["acceptance_result_requirements"] = [
        {
            "acceptance_item_id": "value",
            "allowed_statuses": ["implemented", "partial", "missing", "blocked", "deferred_with_approval"],
            "evidence_required": ["validation_report", "work_proof_marker", "proof-of-behavior-scorecard"],
            "worker_evidence_required": ["validation_report", "work_proof_marker", "proof-of-behavior-scorecard"],
            "lifecycle_evidence_required": ["scope_closure_report"],
        }
    ]
    sample_task["proof_scorecard_required"] = True
    sample_task["required_proof_level"] = "workflow_behavior"
    sample_task["redaction_posture"] = {"classification": "internal", "customer_safe": False}
    validate_runner_ready_task_fields(sample_task, "self-test")
    review_task = dict(sample_task)
    review_task["required_review"] = {
        "required": True,
        "review_type": "architecture",
        "reviewer_class": "peer_agent",
    }
    validate_task_required_review(review_task, "self-test", True)
    validate_architecture_target_paths(
        {
            "architecture_target_paths": ["internal/review/"],
            "path_planning_method": "self_test",
            "allowed_paths": ["internal/review/", "tests/"],
        },
        "self-test",
    )
    validate_lifecycle_path_ownership(
        {
            "work_item_id": "relia-mvp-t1",
            "allowed_paths": [".factory/artifacts/task-runs/T1/"],
            "forbidden_paths": [
                ".factory/artifacts/pr-lifecycle/relia-mvp-t1/",
                ".factory/artifacts/task-runs/T1/scope-closure-report.json",
                ".factory/artifacts/task-runs/T1/scope-closure-map.json",
                ".factory/artifacts/task-runs/T1/factoryd-run-once-report.json",
            ],
            "lifecycle_evidence_required": [
                "scope_closure_report",
                "pr_lifecycle_report",
                "factoryd_run_once_report",
            ],
        },
        "T1",
    )

    original_fail = fail
    original_config_grants = validator.factoryd_config_capability_grants
    original_example_config = validator.FACTORYD_CONFIG
    original_active_config = validator.FACTORYD_ACTIVE_CONFIG
    original_autoship_config = validator.FACTORYD_AUTOSHIP_CONFIG
    globals()["fail"] = lambda message: (_ for _ in ()).throw(AssertionError(message))
    validator.fail = fail
    try:
        missing_runner_ready_task = dict(sample_task)
        del missing_runner_ready_task["required_proof_level"]
        try:
            validate_runner_ready_task_fields(missing_runner_ready_task, "self-test")
        except AssertionError as exc:
            if "missing runner-ready field" not in str(exc):
                raise
        else:
            fail("missing runner-ready proof field fixture did not fail closed")

        try:
            validate_task_required_review(sample_task, "self-test", True)
        except AssertionError as exc:
            if "required_review" not in str(exc):
                raise
        else:
            fail("missing required review fixture did not fail closed")

        lifecycle_allowed_path = {
            "work_item_id": "relia-mvp-t1",
            "allowed_paths": [
                ".factory/artifacts/task-runs/T1/",
                ".factory/artifacts/pr-lifecycle/relia-mvp-t1/",
            ],
            "forbidden_paths": [
                ".factory/artifacts/pr-lifecycle/relia-mvp-t1/",
                ".factory/artifacts/task-runs/T1/scope-closure-report.json",
                ".factory/artifacts/task-runs/T1/scope-closure-map.json",
                ".factory/artifacts/task-runs/T1/factoryd-run-once-report.json",
            ],
            "lifecycle_evidence_required": [
                "scope_closure_report",
                "pr_lifecycle_report",
                "factoryd_run_once_report",
            ],
        }
        try:
            validate_lifecycle_path_ownership(lifecycle_allowed_path, "T1")
        except AssertionError as exc:
            if ".allowed_paths includes daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("lifecycle allowed path fixture did not fail closed")

        lifecycle_root_allowed_path = json.loads(json.dumps(lifecycle_allowed_path))
        lifecycle_root_allowed_path["allowed_paths"] = [
            ".factory/artifacts/task-runs/T1/",
            ".factory/artifacts/pr-lifecycle/",
        ]
        try:
            validate_lifecycle_path_ownership(lifecycle_root_allowed_path, "T1")
        except AssertionError as exc:
            if ".allowed_paths includes daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("lifecycle root allowed path fixture did not fail closed")

        lifecycle_ancestor_allowed_path = json.loads(json.dumps(lifecycle_allowed_path))
        lifecycle_ancestor_allowed_path["allowed_paths"] = [
            ".factory/",
        ]
        try:
            validate_lifecycle_path_ownership(lifecycle_ancestor_allowed_path, "T1")
        except AssertionError as exc:
            if ".allowed_paths includes daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("lifecycle ancestor allowed path fixture did not fail closed")

        task_runs_root_allowed_path = json.loads(json.dumps(lifecycle_allowed_path))
        task_runs_root_allowed_path["allowed_paths"] = [
            ".factory/artifacts/task-runs/",
        ]
        try:
            validate_lifecycle_path_ownership(task_runs_root_allowed_path, "T1")
        except AssertionError as exc:
            if ".allowed_paths includes daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("task-runs root allowed path fixture did not fail closed")

        other_task_run_allowed_path = json.loads(json.dumps(lifecycle_allowed_path))
        other_task_run_allowed_path["allowed_paths"] = [
            ".factory/artifacts/task-runs/T2/",
        ]
        try:
            validate_lifecycle_path_ownership(other_task_run_allowed_path, "T1")
        except AssertionError as exc:
            if ".allowed_paths includes daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("other task-run allowed path fixture did not fail closed")

        lifecycle_missing_forbidden = {
            "work_item_id": "relia-mvp-t1",
            "allowed_paths": [".factory/artifacts/task-runs/T1/"],
            "forbidden_paths": [".factory/artifacts/pr-lifecycle/relia-mvp-t1/"],
            "lifecycle_evidence_required": [
                "scope_closure_report",
                "pr_lifecycle_report",
                "factoryd_run_once_report",
            ],
        }
        try:
            validate_lifecycle_path_ownership(lifecycle_missing_forbidden, "T1")
        except AssertionError as exc:
            if ".forbidden_paths must reserve daemon-owned lifecycle evidence paths" not in str(exc):
                raise
        else:
            fail("missing lifecycle forbidden path fixture did not fail closed")

        nested_lifecycle_missing_forbidden = {
            "work_item_id": "relia-mvp-t1",
            "allowed_paths": [".factory/artifacts/task-runs/T1/"],
            "forbidden_paths": [
                ".factory/artifacts/pr-lifecycle/relia-mvp-t1/",
                ".factory/artifacts/task-runs/T1/scope-closure-report.json",
                ".factory/artifacts/task-runs/T1/scope-closure-map.json",
            ],
            "lifecycle_evidence_required": ["scope_closure_report"],
            "acceptance_result_requirements": [
                {
                    "acceptance_item_id": "A1",
                    "lifecycle_evidence_required": ["factoryd_run_once_report"],
                }
            ],
        }
        try:
            validate_lifecycle_path_ownership(nested_lifecycle_missing_forbidden, "T1")
        except AssertionError as exc:
            if "factoryd-run-once-report.json" not in str(exc):
                raise
        else:
            fail("nested lifecycle forbidden path fixture did not fail closed")

        scope_closure_map_missing_forbidden = {
            "work_item_id": "relia-mvp-t1",
            "allowed_paths": [".factory/artifacts/task-runs/T1/"],
            "forbidden_paths": [],
            "lifecycle_evidence_required": ["scope_closure_map"],
        }
        try:
            validate_lifecycle_path_ownership(scope_closure_map_missing_forbidden, "T1")
        except AssertionError as exc:
            if "scope-closure-map.json" not in str(exc):
                raise
        else:
            fail("scope closure map forbidden path fixture did not fail closed")

        non_list_evidence_required = dict(sample_task)
        non_list_evidence_required["evidence_required"] = "validation_report"
        try:
            validate_runner_ready_task_fields(non_list_evidence_required, "self-test")
        except AssertionError as exc:
            if ".evidence_required must be a non-empty list" not in str(exc):
                raise
        else:
            fail("non-list evidence_required fixture did not fail closed")

        empty_semantic_invariants = dict(sample_task)
        empty_semantic_invariants["semantic_invariants"] = []
        try:
            validate_runner_ready_task_fields(empty_semantic_invariants, "self-test")
        except AssertionError as exc:
            if ".semantic_invariants must be a non-empty list" not in str(exc):
                raise
        else:
            fail("empty semantic_invariants fixture did not fail closed")

        blank_semantic_invariant = dict(sample_task)
        blank_semantic_invariant["semantic_invariants"] = ["preserve behavior", ""]
        try:
            validate_runner_ready_task_fields(blank_semantic_invariant, "self-test")
        except AssertionError as exc:
            if ".semantic_invariants[1] must be a non-empty string" not in str(exc):
                raise
        else:
            fail("blank semantic_invariant fixture did not fail closed")

        duplicate_semantic_invariant = dict(sample_task)
        duplicate_semantic_invariant["semantic_invariants"] = ["preserve behavior", "preserve behavior"]
        try:
            validate_runner_ready_task_fields(duplicate_semantic_invariant, "self-test")
        except AssertionError as exc:
            if ".semantic_invariants must not contain duplicate entries" not in str(exc):
                raise
        else:
            fail("duplicate semantic_invariants fixture did not fail closed")

        non_list_inherited_evidence = json.loads(json.dumps(sample_task))
        non_list_inherited_evidence["validation_contract_inheritance"] = {
            "evidence_required": "validation_report",
            "worker_evidence_required": ["validation_report"],
            "lifecycle_evidence_required": ["scope_closure_report"],
        }
        try:
            validate_runner_ready_task_fields(non_list_inherited_evidence, "self-test")
        except AssertionError as exc:
            if "validation_contract_inheritance.evidence_required must be a list" not in str(exc):
                raise
        else:
            fail("non-list inherited evidence_required fixture did not fail closed")

        misplaced_inherited_evidence = json.loads(json.dumps(sample_task))
        misplaced_inherited_evidence["validation_contract_inheritance"] = {
            "evidence_required": ["validation_report"],
            "worker_evidence_required": ["scope_closure_report"],
            "lifecycle_evidence_required": ["scope_closure_report"],
        }
        try:
            validate_runner_ready_task_fields(misplaced_inherited_evidence, "self-test")
        except AssertionError as exc:
            if "validation_contract_inheritance.worker_evidence_required" not in str(exc):
                raise
        else:
            fail("misplaced inherited worker evidence fixture did not fail closed")

        missing_worker_mirror = json.loads(json.dumps(sample_task))
        missing_worker_mirror["proof_scorecard_required"] = False
        missing_worker_mirror["required_proof_level"] = "source_evidence"
        missing_worker_mirror["evidence_required"] = ["validation_report", "work_proof_marker"]
        missing_worker_mirror["worker_evidence_required"] = ["validation_report"]
        try:
            validate_runner_ready_task_fields(missing_worker_mirror, "self-test")
        except AssertionError as exc:
            if ".worker_evidence_required must mirror worker-owned evidence_required defaults" not in str(exc):
                raise
        else:
            fail("missing worker evidence mirror fixture did not fail closed")

        missing_evidence_mirror = json.loads(json.dumps(sample_task))
        missing_evidence_mirror["proof_scorecard_required"] = False
        missing_evidence_mirror["required_proof_level"] = "source_evidence"
        missing_evidence_mirror["evidence_required"] = ["validation_report"]
        missing_evidence_mirror["worker_evidence_required"] = ["validation_report", "work_proof_marker"]
        missing_evidence_mirror["acceptance_result_requirements"][0]["evidence_required"] = ["validation_report"]
        missing_evidence_mirror["acceptance_result_requirements"][0]["worker_evidence_required"] = ["validation_report"]
        try:
            validate_runner_ready_task_fields(missing_evidence_mirror, "self-test")
        except AssertionError as exc:
            if ".evidence_required must mirror worker-owned worker_evidence_required defaults" not in str(exc):
                raise
        else:
            fail("missing evidence_required mirror fixture did not fail closed")

        missing_inherited_worker_mirror = json.loads(json.dumps(sample_task))
        missing_inherited_worker_mirror["proof_scorecard_required"] = False
        missing_inherited_worker_mirror["required_proof_level"] = "source_evidence"
        missing_inherited_worker_mirror["validation_contract_inheritance"] = {
            "evidence_required": ["validation_report", "work_proof_marker"],
            "worker_evidence_required": ["validation_report"],
            "lifecycle_evidence_required": ["scope_closure_report"],
        }
        try:
            validate_runner_ready_task_fields(missing_inherited_worker_mirror, "self-test")
        except AssertionError as exc:
            if ".validation_contract_inheritance.worker_evidence_required must mirror" not in str(exc):
                raise
        else:
            fail("missing inherited worker evidence mirror fixture did not fail closed")

        missing_top_level_inherited_worker_mirror = json.loads(json.dumps(sample_task))
        missing_top_level_inherited_worker_mirror["proof_scorecard_required"] = False
        missing_top_level_inherited_worker_mirror["required_proof_level"] = "source_evidence"
        missing_top_level_inherited_worker_mirror["evidence_required"] = ["validation_report"]
        missing_top_level_inherited_worker_mirror["worker_evidence_required"] = ["validation_report"]
        missing_top_level_inherited_worker_mirror["validation_contract_inheritance"] = {
            "evidence_required": ["validation_report", "work_proof_marker"],
            "worker_evidence_required": ["validation_report", "work_proof_marker"],
            "lifecycle_evidence_required": ["scope_closure_report"],
        }
        try:
            validate_runner_ready_task_fields(missing_top_level_inherited_worker_mirror, "self-test")
        except AssertionError as exc:
            if ".worker_evidence_required must mirror inherited worker-owned evidence defaults" not in str(exc):
                raise
        else:
            fail("missing top-level inherited worker evidence mirror fixture did not fail closed")

        missing_inherited_lifecycle_mirror = json.loads(json.dumps(sample_task))
        missing_inherited_lifecycle_mirror["proof_scorecard_required"] = False
        missing_inherited_lifecycle_mirror["required_proof_level"] = "source_evidence"
        missing_inherited_lifecycle_mirror["validation_contract_inheritance"] = {
            "evidence_required": ["validation_report"],
            "worker_evidence_required": ["validation_report"],
            "lifecycle_evidence_required": [
                "scope_closure_report",
                "pr_lifecycle_report",
                "factoryd_run_once_report",
            ],
        }
        missing_inherited_lifecycle_mirror["lifecycle_evidence_required"] = ["scope_closure_report"]
        try:
            validate_runner_ready_task_fields(missing_inherited_lifecycle_mirror, "self-test")
        except AssertionError as exc:
            if ".lifecycle_evidence_required must mirror lifecycle-owned evidence defaults" not in str(exc):
                raise
        else:
            fail("missing inherited lifecycle evidence mirror fixture did not fail closed")

        missing_nested_worker_mirror = json.loads(json.dumps(sample_task))
        missing_nested_worker_mirror["proof_scorecard_required"] = False
        missing_nested_worker_mirror["required_proof_level"] = "source_evidence"
        missing_nested_worker_mirror["evidence_required"] = ["validation_report"]
        missing_nested_worker_mirror["worker_evidence_required"] = ["validation_report"]
        missing_nested_worker_mirror["acceptance_result_requirements"][0]["evidence_required"] = [
            "validation_report",
            "work_proof_marker",
        ]
        missing_nested_worker_mirror["acceptance_result_requirements"][0]["worker_evidence_required"] = [
            "validation_report",
        ]
        try:
            validate_runner_ready_task_fields(missing_nested_worker_mirror, "self-test")
        except AssertionError as exc:
            if ".acceptance_result_requirements[0].worker_evidence_required must mirror" not in str(exc):
                raise
        else:
            fail("missing nested worker evidence mirror fixture did not fail closed")

        missing_implied_scorecard = json.loads(json.dumps(sample_task))
        missing_implied_scorecard["proof_scorecard_required"] = False
        missing_implied_scorecard["required_proof_level"] = "workflow_behavior"
        missing_implied_scorecard["evidence_required"] = ["validation_report"]
        missing_implied_scorecard["worker_evidence_required"] = ["validation_report"]
        for requirement in missing_implied_scorecard["acceptance_result_requirements"]:
            requirement["evidence_required"] = ["validation_report"]
            requirement["worker_evidence_required"] = ["validation_report"]
        try:
            validate_runner_ready_task_fields(missing_implied_scorecard, "self-test")
        except AssertionError as exc:
            if "requires a proof-of-behavior scorecard" not in str(exc):
                raise
        else:
            fail("workflow proof level without scorecard fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["internal/review/"],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["internal"],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "broad internal" not in str(exc):
                raise
        else:
            fail("broad internal/ allowed path fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["../../internal/review/"],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["internal/review/"],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "parent-directory segments" not in str(exc):
                raise
        else:
            fail("parent-directory architecture target fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": [123],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["internal/review/"],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "architecture_target_paths[0] path must be a string" not in str(exc):
                raise
        else:
            fail("non-string architecture target fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["internal/review/"],
                    "path_planning_method": "self_test",
                    "allowed_paths": [{"path": "internal/review/"}],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "allowed_paths[0] path must be a string" not in str(exc):
                raise
        else:
            fail("non-string allowed path fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["/tmp/review"],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["tmp/review"],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "repo-relative" not in str(exc):
                raise
        else:
            fail("absolute architecture target fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["./"],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["internal/review/"],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "architecture_target_paths[0] path must not target repository root" not in str(exc):
                raise
        else:
            fail("repo-root architecture target fixture did not fail closed")

        try:
            validate_architecture_target_paths(
                {
                    "architecture_target_paths": ["internal/review/"],
                    "path_planning_method": "self_test",
                    "allowed_paths": ["."],
                },
                "self-test",
            )
        except AssertionError as exc:
            if "allowed_paths[0] path must not target repository root" not in str(exc):
                raise
        else:
            fail("repo-root allowed path fixture did not fail closed")

        try:
            validate_validation_contract_evidence_split(
                {
                    "evidence_required": "validation_report",
                    "worker_evidence_required": ["validation_report"],
                    "lifecycle_evidence_required": ["scope_closure_report"],
                },
                "self-test.validation-contract",
            )
        except AssertionError as exc:
            if ".evidence_required must be a non-empty list" not in str(exc):
                raise
        else:
            fail("validation contract non-list evidence_required fixture did not fail closed")

        try:
            validate_validation_contract_evidence_split(
                {
                    "evidence_required": ["work_proof_markerx"],
                    "worker_evidence_required": ["work_proof_markerx"],
                    "lifecycle_evidence_required": ["scope_closure_report"],
                },
                "self-test.validation-contract",
            )
        except AssertionError as exc:
            if "unsupported evidence keys" not in str(exc) and "unsupported worker evidence keys" not in str(exc):
                raise
        else:
            fail("validation contract unknown evidence key fixture did not fail closed")

        try:
            validate_runner_ready_field_sync(
                {
                    "validation_contract": {
                        "factoryd_runtime_requirements": {
                            "runner_ready_fields": ["allowed_paths"]
                        }
                    }
                },
                {
                    "factoryd_runtime_requirements": {
                        "runner_ready_fields": ["allowed_paths", "semantic_invariants"]
                    }
                },
            )
        except AssertionError as exc:
            if "runner_ready_fields must match" not in str(exc):
                raise
        else:
            fail("runner-ready field sync fixture did not fail closed")

        if validated_remaining_task_refs({"task_refs": ["T1"], "remaining_task_refs": ["T1"]}, {"T1"}, "self-test") != {"T1"}:
            fail("remaining_task_refs valid fixture did not return canonical task refs")
        try:
            validated_remaining_task_refs({"task_refs": ["T1"], "remaining_task_refs": "T1"}, {"T1"}, "self-test")
        except AssertionError as exc:
            if "remaining_task_refs must be a list" not in str(exc):
                raise
        else:
            fail("remaining_task_refs scalar fixture did not fail closed")
        for malformed in ["", False, {}, None]:
            try:
                validated_remaining_task_refs({"task_refs": ["T1"], "remaining_task_refs": malformed}, {"T1"}, "self-test")
            except AssertionError as exc:
                if "remaining_task_refs must be a list" not in str(exc):
                    raise
            else:
                fail("remaining_task_refs falsey malformed fixture did not fail closed")
        try:
            validated_remaining_task_refs({"task_refs": ["T1"], "remaining_task_refs": ["T1 "]}, {"T1"}, "self-test")
        except AssertionError as exc:
            if "canonical task ids" not in str(exc):
                raise
        else:
            fail("remaining_task_refs whitespace fixture did not fail closed")
        try:
            validated_remaining_task_refs({"task_refs": ["T2"], "remaining_task_refs": ["T2"]}, {"T1"}, "self-test")
        except AssertionError as exc:
            if "references missing task" not in str(exc):
                raise
        else:
            fail("remaining_task_refs unknown task fixture did not fail closed")
        try:
            validated_remaining_task_refs({"task_refs": ["T1"], "remaining_task_refs": ["T2"]}, {"T1", "T2"}, "self-test")
        except AssertionError as exc:
            if "remaining_task_refs must be a subset of task_refs" not in str(exc):
                raise
        else:
            fail("remaining_task_refs cross-item fixture did not fail closed")

        active_config_grant = {
            "task_id": "T9",
            "capability": "model_provider_endpoint",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/model_provider_endpoint.md",
            "network_allowlist": ["api.example.com"],
            "provider_identity": "example-provider",
            "provider_model": "example-model",
            "provider_endpoint": "https://api.example.com/v1",
            "credential_environment": "RELIA_PROVIDER_API_KEY",
            "budget_posture": "capped",
            "redaction_posture": "redacted",
            "expires_at": "2099-12-31T23:59:59Z",
        }
        validate_active_capability_grants({"capability_grants": [active_config_grant]})
        invalid_credentials_grant = {
            "task_id": "T9",
            "capability": "credentials",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/credentials.md",
            "credential_scopes": ["env:RELIA_PROVIDER_API_KEY"],
        }
        try:
            validate_active_capability_grants({"capability_grants": [invalid_credentials_grant]})
        except AssertionError as exc:
            if "credential_environment" not in str(exc):
                raise
        else:
            fail("active credentials grant without credential_environment did not fail closed")
        invalid_provider_grant = {
            "task_id": "T9",
            "capability": "model_provider_endpoint",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/model_provider_endpoint.md",
            "credential_environment": "RELIA_PROVIDER_API_KEY",
        }
        try:
            validate_active_capability_grants({"capability_grants": [invalid_provider_grant]})
        except AssertionError as exc:
            if "model_provider_endpoint grant missing fields" not in str(exc):
                raise
        else:
            fail("incomplete active model_provider_endpoint grant did not fail closed")
        expired_grant = {
            "task_id": "T9",
            "capability": "approval",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/T9.md",
            "expires_at": "2026-01-01T00:00:00Z",
        }
        try:
            validate_active_capability_grants({"capability_grants": [expired_grant]})
        except AssertionError as exc:
            if "must be in the future" not in str(exc):
                raise
        else:
            fail("expired active capability grant did not fail closed")
        invalid_artifact_grant = {
            "task_id": "T9",
            "capability": "model_artifact_pull",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/model_artifact_pull.md",
        }
        try:
            validate_active_capability_grants({"capability_grants": [invalid_artifact_grant]})
        except AssertionError as exc:
            if "model_artifact_pull grant missing fields" not in str(exc):
                raise
        else:
            fail("incomplete active model_artifact_pull grant did not fail closed")
        nonexpiring_credentials_grant = {
            "task_id": "T9",
            "capability": "credentials",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/credentials.md",
            "credential_environment": "RELIA_PROVIDER_API_KEY",
            "credential_scopes": ["env:RELIA_PROVIDER_API_KEY"],
        }
        try:
            validate_active_capability_grants({"capability_grants": [nonexpiring_credentials_grant]})
        except AssertionError as exc:
            if "requires expires_at" not in str(exc):
                raise
        else:
            fail("nonexpiring active credentials grant did not fail closed")
        noncanonical_capability_grant = {
            "task_id": "T9",
            "capability": " Credentials ",
            "approved": False,
        }
        try:
            validate_active_capability_grants({"capability_grants": [noncanonical_capability_grant]})
        except AssertionError as exc:
            if "canonical lowercase value" not in str(exc):
                raise
        else:
            fail("noncanonical active capability value did not fail closed")
        with TemporaryDirectory() as temp_dir:
            temp_root = Path(temp_dir)
            example_config = temp_root / "factoryd.example.json"
            active_config = temp_root / "factoryd.json"
            autoship_config = temp_root / "factoryd.autoship.example.json"
            config_payload = {"repos": {FACTORYD_REPO_KEY: {"capability_grants": [active_config_grant]}}}
            empty_active_payload = {"repos": {FACTORYD_REPO_KEY: {"capability_grants": []}}}
            example_config.write_text(json.dumps(config_payload), encoding="utf-8")
            autoship_config.write_text(json.dumps(config_payload), encoding="utf-8")
            active_config.write_text(json.dumps(empty_active_payload), encoding="utf-8")
            try:
                validator.FACTORYD_CONFIG = example_config
                validator.FACTORYD_ACTIVE_CONFIG = active_config
                validator.FACTORYD_AUTOSHIP_CONFIG = autoship_config
                if factoryd_config_capability_grants():
                    fail("example and autoship config grants should be ignored")
                active_config.write_text(json.dumps(config_payload), encoding="utf-8")
                if factoryd_config_capability_grants() != [active_config_grant]:
                    fail("active factoryd.json grants should be visible")
                if active_factoryd_repo_config(FACTORYD_REPO_KEY) != config_payload["repos"][FACTORYD_REPO_KEY]:
                    fail("repo-shaped active factoryd.json config should be visible")
                validate_active_architecture_budget_policy(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                metadata_overlay_payload = {
                    "repos": {
                        FACTORYD_REPO_KEY: {
                            "repo_path": str(temp_root),
                            "capability_grants": [active_config_grant],
                        }
                    }
                }
                active_config.write_text(json.dumps(metadata_overlay_payload), encoding="utf-8")
                if factoryd_config_capability_grants() != [active_config_grant]:
                    fail("metadata-bearing active factoryd.json grants should be visible")
                validate_active_architecture_budget_policy(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                top_level_grants_payload = {"capability_grants": [active_config_grant]}
                active_config.write_text(json.dumps(top_level_grants_payload), encoding="utf-8")
                if factoryd_config_capability_grants() != [active_config_grant]:
                    fail("top-level active factoryd.json grants should remain visible")
                if active_factoryd_repo_config(FACTORYD_REPO_KEY) is not None:
                    fail("top-level-only active factoryd.json config should skip repo architecture parity")
                validate_active_capability_grants(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                invalid_top_level_payload = {"capability_grants": [invalid_credentials_grant]}
                active_config.write_text(json.dumps(invalid_top_level_payload), encoding="utf-8")
                try:
                    validate_active_capability_grants(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                except AssertionError as exc:
                    if "credential_environment" not in str(exc):
                        raise
                else:
                    fail("top-level active credentials grant without credential_environment did not fail closed")
                malformed_top_level_payload = {"capability_grants": ["bad"]}
                active_config.write_text(json.dumps(malformed_top_level_payload), encoding="utf-8")
                try:
                    validate_active_capability_grants(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                except AssertionError as exc:
                    if "must be an object" not in str(exc):
                        raise
                else:
                    fail("malformed top-level active capability grant did not fail closed")
                full_active_missing_budget_payload = {
                    "repos": {
                        FACTORYD_REPO_KEY: {
                            "acceptance_ledger": ".factory/artifacts/prd-to-plan/relia-mvp/acceptance-ledger.json",
                            "task_packets": ".factory/artifacts/prd-to-plan/relia-mvp/task-packets.json",
                            "scope_closure_map": ".factory/artifacts/prd-to-plan/relia-mvp/scope-closure-map.json",
                            "validation_contract": ".factory/artifacts/prd-to-plan/relia-mvp/validation-contract.json",
                            "validation_commands": ["make prepush-full"],
                            "worker_type": "codex_cli",
                            "capability_grants": [],
                        }
                    }
                }
                active_config.write_text(json.dumps(full_active_missing_budget_payload), encoding="utf-8")
                try:
                    validate_active_architecture_budget_policy(active_factoryd_repo_config(FACTORYD_REPO_KEY))
                except AssertionError as exc:
                    if "architecture_budget" not in str(exc):
                        raise
                else:
                    fail("full active factoryd.json config without architecture_budget should fail closed")
            finally:
                validator.FACTORYD_CONFIG = original_example_config
                validator.FACTORYD_ACTIVE_CONFIG = original_active_config
                validator.FACTORYD_AUTOSHIP_CONFIG = original_autoship_config

        validate_context_brief(
            {
                "alignment_decisions": {
                    "model_provider_endpoint": {
                        "required_grant": "model_provider_endpoint",
                        "generic_grants_sufficient": False,
                        "required_before_dispatch": ["T9"],
                    }
                }
            },
            {"T9"},
        )
        try:
            validate_context_brief(
                {
                    "alignment_decisions": {
                        "model_provider_endpoint": {
                            "required_grant": "model_provider_endpoint",
                            "generic_grants_sufficient": False,
                            "required_before_dispatch": [],
                        }
                    }
                },
                {"T9"},
            )
        except AssertionError as exc:
            if "required_before_dispatch missing provider-gated tasks" not in str(exc):
                raise
        else:
            fail("missing provider dispatch context fixture did not fail closed")

        active_wildcard_task = model_provider_gate_task()
        active_wildcard_task["factoryd_runtime"]["capability_grants"] = []
        active_wildcard_grant = {
            "task_id": "*",
            "capability": "model_provider_endpoint",
            "approved": True,
            "evidence_ref": ".factory/artifacts/approvals/model_provider_endpoint.md",
            "network_allowlist": ["api.example.com"],
            "provider_identity": "example-provider",
            "provider_model": "example-model",
            "provider_endpoint": "https://api.example.com/v1",
            "credential_environment": "RELIA_PROVIDER_API_KEY",
            "budget_posture": "capped",
            "redaction_posture": "redacted",
        }
        validator.factoryd_config_capability_grants = lambda: [active_wildcard_grant]
        try:
            validate_model_provider_gate(active_wildcard_task)
        except AssertionError as exc:
            if "active model_provider_endpoint grants must be task-scoped" not in str(exc):
                raise
        else:
            fail("active wildcard model-provider grant fixture did not fail closed")
        validator.factoryd_config_capability_grants = original_config_grants

        pending_base_url_task = model_provider_gate_task("T9", "T9")
        pending_base_url_task["factoryd_runtime"]["capability_grants"] = []
        pending_base_url_grant = {
            "task_id": "T9",
            "capability": "model_provider_endpoint",
            "evidence_ref": ".factory/artifacts/approvals/model_provider_endpoint.md",
        }
        pending_base_url_grant.update(
            {
                "approved": True,
                "network_allowlist": ["api.example.com"],
                "provider_identity": "example-provider",
                "provider_model": "example-model",
                "provider_endpoint": "   ",
                "base_url": "pending-approved-base-url",
                "credential_environment": "RELIA_PROVIDER_API_KEY",
                "budget_posture": "capped",
                "redaction_posture": "redacted",
            }
        )
        validator.factoryd_config_capability_grants = lambda: [pending_base_url_grant]
        try:
            try:
                validate_model_provider_gate(pending_base_url_task)
            except AssertionError as exc:
                if "pending placeholders" not in str(exc):
                    raise
            else:
                fail("whitespace endpoint with pending base_url fixture did not fail closed")
        finally:
            validator.factoryd_config_capability_grants = original_config_grants

        pending_extra_base_url_task = model_provider_gate_task("T9", "T9")
        pending_extra_base_url_task["factoryd_runtime"]["capability_grants"] = []
        pending_extra_base_url_grant = dict(active_config_grant)
        pending_extra_base_url_grant["base_url"] = "pending-approved-base-url"
        validator.factoryd_config_capability_grants = lambda: [pending_extra_base_url_grant]
        try:
            try:
                validate_model_provider_gate(pending_extra_base_url_task)
            except AssertionError as exc:
                if "pending placeholders" not in str(exc):
                    raise
            else:
                fail("real endpoint with pending base_url fixture did not fail closed")
        finally:
            validator.factoryd_config_capability_grants = original_config_grants

        approved_seed_task = model_provider_gate_task("T9", "T9")
        approved_seed_grant = approved_seed_task["factoryd_runtime"]["capability_grants"][0]
        approved_seed_grant.update(
            {
                "approved": True,
                "network_allowlist": ["api.example.com"],
                "provider_identity": "example-provider",
                "provider_model": "example-model",
                "provider_endpoint": "https://api.example.com/v1",
                "credential_environment": "RELIA_PROVIDER_API_KEY",
                "budget_posture": "capped",
                "redaction_posture": "redacted",
            }
        )
        try:
            validate_model_provider_gate(approved_seed_task)
        except AssertionError as exc:
            if "seed model_provider_endpoint grants must stay approved false" not in str(exc):
                raise
        else:
            fail("approved seed model-provider grant fixture did not fail closed")

        non_string_allowlist_task = model_provider_gate_task("T9", "T9")
        non_string_allowlist_grant = non_string_allowlist_task["factoryd_runtime"]["capability_grants"][0]
        non_string_allowlist_grant["network_allowlist"] = [{"host": "api.example.com"}]
        try:
            validate_model_provider_gate(non_string_allowlist_task)
        except AssertionError as exc:
            if "network_allowlist must be a non-empty string list" not in str(exc):
                raise
        else:
            fail("non-string provider allowlist fixture did not fail closed")

        non_string_metadata_task = model_provider_gate_task("T9", "T9")
        non_string_metadata_task["factoryd_runtime"]["capability_grants"] = []
        non_string_metadata_grant = dict(active_config_grant)
        non_string_metadata_grant["provider_model"] = {"name": "example-model"}
        validator.factoryd_config_capability_grants = lambda: [non_string_metadata_grant]
        try:
            try:
                validate_model_provider_gate(non_string_metadata_task)
            except AssertionError as exc:
                if "fields must be non-empty strings" not in str(exc):
                    raise
            else:
                fail("non-string provider metadata fixture did not fail closed")
        finally:
            validator.factoryd_config_capability_grants = original_config_grants

        missing_requirement_field_task = model_provider_gate_task("T9", "T9")
        missing_requirement_field_task["model_provider_requirements"]["required_fields"].remove("network_allowlist")
        try:
            validate_model_provider_gate(missing_requirement_field_task)
        except AssertionError as exc:
            if "required_fields missing" not in str(exc):
                raise
        else:
            fail("missing model-provider requirement field fixture did not fail closed")

        missing_seed_metadata_task = model_provider_gate_task("T9", "T9")
        missing_seed_metadata_task["factoryd_runtime"]["capability_grants"] = [
            {
                "task_id": "T9",
                "capability": "model_provider_endpoint",
                "approved": False,
            }
        ]
        validator.factoryd_config_capability_grants = lambda: [active_config_grant]
        try:
            try:
                validate_model_provider_gate(missing_seed_metadata_task)
            except AssertionError as exc:
                if "seed model_provider_endpoint grant missing fields" not in str(exc):
                    raise
            else:
                fail("missing seed provider metadata fixture did not fail closed")
        finally:
            validator.factoryd_config_capability_grants = original_config_grants
    finally:
        validator.factoryd_config_capability_grants = original_config_grants
        globals()["fail"] = original_fail
        validator.fail = original_fail
    print("repo-pack validator self-test passed")
