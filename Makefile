GO ?= go
RELIA_GOCACHE ?= /tmp/relia-go-build
GO_TEST_ENV := GOCACHE='$(RELIA_GOCACHE)'
PKG_LIST := ./cmd/...
COVERAGE_MIN ?= 75

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
	env $(GO_TEST_ENV) $(GO) test ./... -count=1

test-coverage:
	mkdir -p .factory/tmp
	env $(GO_TEST_ENV) $(GO) test $(PKG_LIST) -count=1 -covermode=atomic -coverprofile=.factory/tmp/coverage.out
	env $(GO_TEST_ENV) python3 scripts/check_go_coverage.py .factory/tmp/coverage.out $(COVERAGE_MIN)

test-contracts:
	python3 scripts/validate_repo_pack.py

prepush-full: lint-fast test-fast test-coverage test-contracts
