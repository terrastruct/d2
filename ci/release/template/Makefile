.POSIX:
.SILENT:

.PHONY: all
all:
	( . ./scripts/lib.sh && echoerr "You must provide a target of install or uninstall for this Makefile" )
	exit 1

.PHONY: install
install:
	./scripts/install.sh

.PHONY: uninstall
uninstall:
	./scripts/uninstall.sh
