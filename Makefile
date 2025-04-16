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
	cd d2js/js && NPM_VERSION="${NPM_VERSION}" prefix "$@" ./make.sh all

SVGDIR := testdata/examples/svg
SVGS = $(shell ./d2 themes | gawk -F':' '/^-/{ printf "$(SVGDIR)/themex-%03d.svg ",$$2 }' || :)

.PHONY: clean
clean:
	rm -f $(SVGS) d2
	rmdir $(SVGDIR)

.PHONY: themesdemo
themesdemo: $(SVGS) d2

$(SVGDIR)/themex-%.svg: testdata/examples/themex.d2 
	$(info Building $@ from $< ...)
	./d2 -t $$(( 10#$* )) $< $@

d2: build
