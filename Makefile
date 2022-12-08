.POSIX:

.PHONY: all
all: fmt gen lint build test

.PHONY: fmt
fmt:
	prefix "$@" ./ci/sub/bin/fmt.sh
.PHONY: gen
gen:
	prefix "$@" ./ci/gen.sh
.PHONY: lint
lint:
	prefix "$@" go vet --composites=false ./...
.PHONY: build
build:
	prefix "$@" go build ./...
.PHONY: test
test:
	prefix "$@" ./ci/test.sh
.PHONY: race
race:
	prefix "$@" ./ci/test.sh --race ./...
