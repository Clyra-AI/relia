#!/usr/bin/env python3
from pathlib import Path
import json
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
FACTORYD_CONFIG = ROOT / ".factory" / "factoryd.example.json"
FACTORYD_ACTIVE_CONFIG = ROOT / ".factory" / "factoryd.json"
FACTORYD_AUTOSHIP_CONFIG = ROOT / ".factory" / "factoryd.autoship.example.json"
FACTORYD_REPO_KEY = "relia"

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

def factoryd_config_capability_grants():
    grants = []
    for path in [FACTORYD_CONFIG, FACTORYD_ACTIVE_CONFIG, FACTORYD_AUTOSHIP_CONFIG]:
        if not path.exists():
            continue
        config = load_json_file(path)
        repos = config.get("repos")
        if isinstance(repos, dict):
            repo = repos.get(FACTORYD_REPO_KEY)
            if isinstance(repo, dict) and isinstance(repo.get("capability_grants"), list):
                grants.extend(grant for grant in repo["capability_grants"] if isinstance(grant, dict))
        elif isinstance(config.get("capability_grants"), list):
            grants.extend(grant for grant in config["capability_grants"] if isinstance(grant, dict))
    return grants

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


def validate_model_provider_gate(task):
    task_id = task.get("task_id") or "T9"
    if task.get("requires_model_provider_endpoint") is not True:
        fail(f"{task_id}.requires_model_provider_endpoint must be true for provider-backed distill work")
    requirements = task.get("model_provider_requirements")
    if not isinstance(requirements, dict) or requirements.get("required_grant") != "model_provider_endpoint":
        fail(f"{task_id}.model_provider_requirements must require model_provider_endpoint")
    provider_surfaces = {str(value) for value in requirements.get("provider_surfaces") or []}
    missing_surfaces = {"openai_compatible_http", "anthropic_messages_http"} - provider_surfaces
    if missing_surfaces:
        fail(f"{task_id}.model_provider_requirements.provider_surfaces missing {sorted(missing_surfaces)}")
    required_fields = {str(value) for value in requirements.get("required_fields") or []}
    if "provider_model" not in required_fields:
        fail(f"{task_id}.model_provider_requirements.required_fields must include provider_model")
    if task_id == "T9":
        for key in ["requires_human_approval", "requires_network", "requires_credentials"]:
            if task.get(key) is not True:
                fail(f"{task_id}.{key} must remain true because T9 mixes model-provider, network, credential, API, webhook, and hosted-service work")
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
        fail(f"{task_id}.seed model_provider_endpoint grants must stay approved false; active approvals belong in .factory/factoryd*.json")
    active_matching = [
        grant for grant in active_grants
        if isinstance(grant, dict)
        and str(grant.get("task_id", "")).strip() == task_id
        and grant.get("capability") == "model_provider_endpoint"
    ]
    matching = [*seed_matching, *active_matching]
    if not matching:
        fail(f"{task_id} must include one seed wildcard or task-scoped model_provider_endpoint grant in factoryd_runtime.capability_grants, or one task-scoped active .factory/factoryd*.json config grant")
    grant = next((candidate for candidate in matching if candidate.get("approved") is True), matching[0])
    approved = grant.get("approved")
    if approved not in (False, True):
        fail(f"{task_id}.model_provider_endpoint grant approved flag must be true or false")
    required_grant_fields = [
        "evidence_ref",
        "network_allowlist",
        "provider_identity",
        "provider_model",
        "credential_environment",
        "budget_posture",
        "redaction_posture",
    ]
    missing = [field for field in required_grant_fields if field not in grant or missing_grant_value(grant[field])]
    if missing:
        fail(f"{task_id}.model_provider_endpoint grant missing fields: {missing}")
    allowlist = grant.get("network_allowlist")
    if not isinstance(allowlist, list) or not all(isinstance(item, str) and item.strip() for item in allowlist):
        fail(f"{task_id}.model_provider_endpoint grant network_allowlist must be a non-empty string list")
    provider_endpoint = str(grant.get("provider_endpoint", "")).strip()
    base_url = str(grant.get("base_url", "")).strip()
    provider_endpoint_or_base_url = provider_endpoint or base_url
    if not provider_endpoint_or_base_url:
        fail(f"{task_id}.model_provider_endpoint grant must include provider_endpoint or base_url")
    if approved is True:
        checked_values = [
            grant.get("provider_identity"),
            grant.get("provider_model"),
            provider_endpoint_or_base_url,
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


def model_provider_gate_task(task_id="T7", grant_task_id="*"):
    return {
        "task_id": task_id,
        "requires_model_provider_endpoint": True,
        "requires_human_approval": False,
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
    if duplicate_values(["T1", "T2", "T1", "T2", "T3"]) != ["T1", "T2"]:
        fail("duplicate_values must preserve duplicate ids in first duplicate order")
    self_test_public_release_boundary()
    validate_model_provider_gate(model_provider_gate_task())

    original_fail = fail
    original_config_grants = factoryd_config_capability_grants
    globals()["fail"] = lambda message: (_ for _ in ()).throw(AssertionError(message))
    try:
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
        globals()["factoryd_config_capability_grants"] = lambda: [active_wildcard_grant]
        try:
            validate_model_provider_gate(active_wildcard_task)
        except AssertionError as exc:
            if "active model_provider_endpoint grants must be task-scoped" not in str(exc):
                raise
        else:
            fail("active wildcard model-provider grant fixture did not fail closed")
        globals()["factoryd_config_capability_grants"] = original_config_grants

        pending_base_url_task = model_provider_gate_task("T7", "T7")
        pending_base_url_task["factoryd_runtime"]["capability_grants"] = []
        pending_base_url_grant = {
            "task_id": "T7",
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
        globals()["factoryd_config_capability_grants"] = lambda: [pending_base_url_grant]
        try:
            try:
                validate_model_provider_gate(pending_base_url_task)
            except AssertionError as exc:
                if "pending placeholders" not in str(exc):
                    raise
            else:
                fail("whitespace endpoint with pending base_url fixture did not fail closed")
        finally:
            globals()["factoryd_config_capability_grants"] = original_config_grants

        approved_seed_task = model_provider_gate_task("T7", "T7")
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

        non_string_allowlist_task = model_provider_gate_task("T7", "T7")
        non_string_allowlist_grant = non_string_allowlist_task["factoryd_runtime"]["capability_grants"][0]
        non_string_allowlist_grant["network_allowlist"] = [{"host": "api.example.com"}]
        try:
            validate_model_provider_gate(non_string_allowlist_task)
        except AssertionError as exc:
            if "network_allowlist must be a non-empty string list" not in str(exc):
                raise
        else:
            fail("non-string provider allowlist fixture did not fail closed")
    finally:
        globals()["factoryd_config_capability_grants"] = original_config_grants
        globals()["fail"] = original_fail
    print("repo-pack validator self-test passed")

def main():
    if "--self-test" in sys.argv[1:]:
        self_test()
        return
    root = ROOT
    for rel in REQUIRED:
        if not (root / rel).exists():
            fail(f"missing required repo-pack file: {rel}")
    cfg = json.loads((root / ".factory/factoryd.example.json").read_text())
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
    for rel in [repo["acceptance_ledger"], repo["task_packets"], repo["scope_closure_map"], repo["validation_contract"]]:
        if not (root / rel).exists():
            fail(f"factoryd config references missing file: {rel}")
    packets = json.loads((root / repo["task_packets"]).read_text())
    plan_dir = (root / repo["task_packets"]).parent
    plan_dir_ref = str(Path(repo["task_packets"]).parent).replace("\\", "/")
    execution_plan_path = plan_dir / "execution-plan.json"
    if not execution_plan_path.exists():
        fail("execution-plan.json is required next to task-packets.json")
    execution_plan = json.loads(execution_plan_path.read_text())
    validation_contract = json.loads((root / repo["validation_contract"]).read_text())
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
    validate_public_release_boundary([
        ("execution-plan", execution_plan),
        ("task-packets", packets),
        ("validation-contract", validation_contract),
        ("acceptance-mapping", mapping),
        ("scope-closure-map", closure),
    ])
    for index, task in enumerate(tasks):
        task_id = task.get("task_id") or f"task[{index}]"
        for key in ["task_id", "objective", "allowed_paths", "forbidden_paths", "validation_commands", "baseline_commands", "red_first_commands", "final_validation_commands", "acceptance_result_requirements", "evidence_required", "stop_conditions", "worker_type", "factoryd_runtime", "required_worker_chain", "lifecycle_gates", "test_matrix_refs", "ci_lane_refs", "ci_control_refs", "coverage_policy_refs", "security_scanner_gates", "engineering_policy_refs", "architecture_guidance_refs", "changelog_intent", "versioning_impact", "migration_impact", "docs_sync_refs", "acceptance_group_id", "acceptance_ledger_ref", "acceptance_item_ids", "alignment_gate_ref", "plan_drift_policy_ref"]:
            if key not in task or task[key] in (None, "", []):
                fail(f"{task_id} missing runner-ready field: {key}")
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
        normalized_allowed_paths = set()
        for path in allowed_paths:
            parts = []
            for part in path.replace("\\", "/").split("/"):
                if part in ("", "."):
                    continue
                if part == "..":
                    if parts:
                        parts.pop()
                    else:
                        parts.append(part)
                    continue
                parts.append(part)
            normalized_allowed_paths.add("/".join(parts))
        control_allowed = sorted(
            path for path in normalized_allowed_paths
            if path == ".factory/artifacts"
            or path == ".factory/artifacts/"
            or path == plan_dir_ref
            or path.startswith(plan_dir_ref + "/")
        )
        if control_allowed:
            fail(f"{task_id}.allowed_paths includes runtime-owned control artifact: {control_allowed}")
        if task.get("worker_type") != "codex_cli":
            fail(f"{task_id}.worker_type must be codex_cli")
        runtime = task.get("factoryd_runtime")
        if not isinstance(runtime, dict) or "capability_grants" not in runtime or not isinstance(runtime["capability_grants"], list):
            fail(f"{task_id}.factoryd_runtime.capability_grants must be a list")
        provider_acceptance_ids = {"MVP-IN-SCOPE-010", "MVP-IN-SCOPE-011"}
        if task.get("requires_model_provider_endpoint") is True or provider_acceptance_ids & task_item_ids:
            validate_model_provider_gate(task)
        scanner = task.get("security_scanner_gates") or {}
        if profile_visibility == "public":
            if scanner.get("required") is not True or scanner.get("status_check") != "CodeQL analyze":
                fail(f"{task_id}.security_scanner_gates must require CodeQL analyze for public repos")
    print("ok: repo pack")

if __name__ == "__main__":
    main()
