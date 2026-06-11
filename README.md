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
	