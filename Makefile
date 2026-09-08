.POSIX:

.PHONY: all
all: fmt gen js lint build test test-wasm

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
.PHONY: test-wasm
test-wasm: fmt
	prefix "$@" ./ci/test-wasm.sh
.PHONY: race
race: fmt
	prefix "$@" ./ci/test.sh --race ./...
.PHONY: js
js: gen
	cd d2js/js && NPM_VERSION="${NPM_VERSION}" prefix "$@" ./make.sh all
