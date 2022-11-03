.POSIX:

.PHONY: all
all:
ifdef CI
	git -c color.ui=always diff --exit-code
endif

ifdef CI_FMT_GO
all: go
endif
.PHONY: go
go: gofmt goimports
.PHONY: gofmt
gofmt:
	prefix "$@" xargsd '\.go$$' gofmt -s -w
.PHONY: goimports
goimports: gofmt
	prefix "$@" xargsd '\.go$$' go run golang.org/x/tools/cmd/goimports@v0.1.12 \
		-w -local="$$CI_GOIMPORTS_LOCAL"

ifdef CI_FMT_PRETTIER
all: prettier
endif
.PHONY: prettier
prettier:
	prefix "$@" xargsd '\.\(js\|jsx\|ts\|tsx\|scss\|css\|html\)$$' \
	  npx prettier@2.7.1 --print-width=90 --write

ifdef CI_FMT_MARKDOWN
all: markdown-toc
endif
.PHONY: markdown-toc
markdown-toc:
	XARGS_N=1 prefix "$@" xargsd '\.md$$' npx markdown-toc@1.2.0 -i

ifdef CI_FMT_GO_MODULE
all: gomodtidy
endif
.PHONY: gomodtidy
gomodtidy:
	prefix "$@" go mod tidy

ifdef CI_FMT_NODE_MODULE
all: yarn
endif
.PHONY: yarn
yarn:
	prefix "$@" yarn $${CI:+--immutable} $${CI:+--immutable-cache}
