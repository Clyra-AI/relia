.PHONY: lint-fast test-fast test-contracts prepush-full

lint-fast:
	test -f AGENTS.md
	test -f WORKFLOW.md
	test -f docs/product/prd.md
	test -f docs/dev/dev_guides.md
	test -f docs/architecture/architecture_guides.md
	test -f .factory/factoryd.example.json
	test -f .factory/factoryd.autoship.example.json
	test -f .tool-versions
	grep -q '^golang 1.26.4$$' .tool-versions

test-fast:
	go test ./...

test-contracts:
	python3 scripts/validate_repo_pack.py

prepush-full: lint-fast test-fast test-contracts
