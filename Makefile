.POSIX:

.PHONY: all
all: fmt gen lint build test

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
	prefix "$@" go build -tags plugins_embed,plugins_embed_dagre,plugins_embed_elk ./...
.PHONY: test
test: fmt
	prefix "$@" ./ci/test.sh
.PHONY: race
race: fmt
	prefix "$@" ./ci/test.sh --race ./...

.PHONY: build-noembed
	prefix "$@" go build ./...
