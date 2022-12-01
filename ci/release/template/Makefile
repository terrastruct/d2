.POSIX:
.SILENT:

.PHONY: all
all:
	(. ./scripts/lib.sh && echoerr "You must provide a target of install or uninstall for this Makefile")
	exit 1

PREFIX = $(DESTDIR)/usr/local

.PHONY: install
install:
	PREFIX='$(PREFIX)' ./scripts/install.sh

.PHONY: uninstall
uninstall:
	PREFIX='$(PREFIX)' ./scripts/uninstall.sh
