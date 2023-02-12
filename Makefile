.POSIX:

.PHONY: all
all: fmt gen lint build test

.PHONY: fmt
fmt:
	# Unset GITHUB_TOKEN, see https://github.com/terrastruct/d2/commit/335d925b7c937d4e7cac7e26de993f60840eb116#commitcomment-98101131
	prefix "$@" GITHUB_TOKEN= ./ci/sub/bin/fmt.sh
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
