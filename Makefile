.POSIX:

.PHONY: all
all: fmt gen js lint build test

.PHONY: fmt
fmt:
	prefix "$@" ./ci/sub/bin/fmt.sh
.PHONY: gen
gen: fmt
	prefix "$@" ./ci/gen.sh
.PHONY: lint
lint: fmt
	prefix "$@" go vet --composites=false ./...
.PHONY: build
build: fmt
	prefix "$@" go build ./...
.PHONY: test
test: fmt
	prefix "$@" ./ci/test.sh
.PHONY: race
race: fmt
	prefix "$@" ./ci/test.sh --race ./...
.PHONY: js
js: gen
	echo "DEBUG: Root Makefile NPM_VERSION=${NPM_VERSION:-not set}"
	cd d2js/js && NPM_VERSION="${NPM_VERSION}" prefix "$@" ./make.sh all
