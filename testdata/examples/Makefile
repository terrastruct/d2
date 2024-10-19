D2 := ../../d2

SVGS = $(shell $(D2) themes | gawk -F':' '/^-/{ printf "out/themex-%03d.svg ",$$2 }' || :)

.PHONY:
all: $(D2) $(SVGS)

$(D2):
	cd $(dir $@) && go build
	$(MAKE) $(SVGS)
# D2 as dependency to expire results if recompiled
out/themex-%.svg: themex.d2 $(D2)
	$(info Building $@ from $< ...)
	$(D2) -t $$(( 10#$* )) $< $@
