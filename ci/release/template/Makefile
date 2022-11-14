.POSIX:
.SILENT:

PREFIX = $(DESTDIR)/usr/local

.PHONY: install
install:
	PREFIX='$(PREFIX)' ./scripts/install.sh

.PHONY: uninstall
uninstall:
	PREFIX='$(PREFIX)' ./scripts/uninstall.sh
