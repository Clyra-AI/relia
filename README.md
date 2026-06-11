# Relia

Generated Factory-ready repository.

## Start

	~~~sh
	make prepush-full
	FACTORY_REPO=/path/to/factory factoryd doctor --config .factory/factoryd.example.json --repo relia --json
	FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.example.json --repo relia --dry-run --json
	# After branch protection, CI, and review gates are proven:
	FACTORY_REPO=/path/to/factory factoryd run --config .factory/factoryd.autoship.example.json --repo relia --loop --max-tasks 1 --json
	~~~

## Post-PRD audit or review findings

Save material findings from `app-audit` or `code-review` as repo-local markdown
under `product/audits/` or `product/reviews/`, then ingest them:

~~~sh
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind audit --input product/audits/<mission>.md --mission <mission> --json
FACTORY_REPO=/path/to/factory factoryd ingest --config .factory/factoryd.example.json --repo relia --kind review --input product/reviews/<mission>.md --mission <mission> --json
~~~
