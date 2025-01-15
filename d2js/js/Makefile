.POSIX:
.PHONY: all
all: fmt build test cleanup

.PHONY: fmt
fmt: node_modules
	prefix "$@" ../../ci/sub/bin/fmt.sh
	prefix "$@" rm -f yarn.lock

.PHONY: build
build: fmt
	prefix "$@" ./ci/build.sh

.PHONY: dev
dev: build
	prefix "$@" git checkout -- src/platform.js src/worker.js
	prefix "$@" bun run dev

.PHONY: test
test: build
	prefix "$@" bun test:all

.PHONY: node_modules
node_modules:
	prefix "$@" bun install $${CI:+--frozen-lockfile}

.PHONY: cleanup
cleanup: test
	prefix "$@" git checkout -- src/platform.js src/worker.js
