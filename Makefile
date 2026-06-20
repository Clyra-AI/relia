GO ?= go
PKG_LIST := ./cmd/...
COVERAGE_MIN ?= 75
TMPDIR ?= /tmp
export GOCACHE ?= $(TMPDIR)/relia-go-build

.PHONY: lint-fast test-fast test-coverage test-contracts prepush-full

lint-fast:
	test -f AGENTS.md
	test -f WORKFLOW.md
	test -f docs/product/prd.md
	test -f docs/dev/dev_guides.md
	test -f docs/architecture/architecture_guides.md
	test -f .factory/factoryd.example.json
	test -f .factory/factoryd.autoship.example.json
	test -f scripts/check_go_coverage.py
	test -f .tool-versions
	grep -q '^golang 1.26.4$$' .tool-versions
	grep -q 'make test-coverage' docs/dev/dev_guides.md

test-fast:
	$(GO) test ./... -count=1

test-coverage:
	mkdir -p .factory/tmp
	$(GO) test $(PKG_LIST) -count=1 -covermode=atomic -coverprofile=.factory/tmp/coverage.out
	python3 scripts/check_go_coverage.py .factory/tmp/coverage.out $(COVERAGE_MIN)

test-contracts:
	python3 scripts/validate_repo_pack.py

prepush-full: lint-fast test-fast test-coverage test-contracts
