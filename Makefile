.POSIX:

.PHONY: all
all: fmt lint build test
ifdef CI
all: assert-linear
endif

.PHONY: fmt
fmt:
	prefix "$@" ./ci/sub/fmt/make.sh
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
.PHONY: assert-linear
assert-linear:
	prefix "$@" ./ci/sub/assert_linear.sh
