.POSIX:
.PHONY: all
all: fmt build test

.PHONY: fmt
fmt: node_modules
	prefix "$@" ../../ci/sub/bin/fmt.sh
	prefix "$@" rm -f yarn.lock

.PHONY: build
build: node_modules
	prefix "$@" ./ci/build.sh

.PHONY: test
test: build
	prefix "$@" bun test:all

.PHONY: node_modules
node_modules:
	prefix "$@" bun install $${CI:+--frozen-lockfile}
