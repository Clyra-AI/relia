#!/usr/bin/env python3
from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from tempfile import TemporaryDirectory

import validate_repo_pack as validator
from repo_pack_architecture import (
    architecture_budget_unexcepted_failures,
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
validate_context_brief = validator.validate_context_brief
validate_architecture_target_paths = validator.validate_architecture_target_paths
validate_lifecycle_path_ownership = validator.validate_lifecycle_path_ownership
validate_model_provider_gate = validator.validate_model_provider_gate
validate_runner_ready_task_fields = validator.validate_runner_ready_task_fields
validate_validation_contract_evidence_split = validator.validate_validation_contract_evidence_split

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
        }
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
                top_level_grants_payload = {"capability_grants": [active_config_grant]}
                active_config.write_text(json.dumps(top_level_grants_payload), encoding="utf-8")
                if factoryd_config_capability_grants() != [active_config_grant]:
                    fail("top-level active factoryd.json grants should remain visible")
                if active_factoryd_repo_config(FACTORYD_REPO_KEY) is not None:
                    fail("top-level-only active factoryd.json config should skip repo architecture parity")
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
