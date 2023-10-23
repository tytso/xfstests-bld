SCRIPTS =	android-xfstests \
		gce-xfstests \
		kvm-xfstests \
		qemu-xfstests

KBUILD_SCRIPTS = kbuild kbuild32 install-kconfig

prefix= $(HOME)/bin

all: $(SCRIPTS) $(KBUILD_SCRIPTS)

clean:
	rm -f $(SCRIPTS) $(KBUILD_SCRIPTS)

install: $(SCRIPTS) $(KBUILD_SCRIPTS)
	for i in $(SCRIPTS) $(KBUILD_SCRIPTS) ; do \
		rm -f $(DESTDIR)$(prefix)/$$i ; \
		install -D $$i $(DESTDIR)$(prefix)/$$i; \
	done

$(SCRIPTS): %: run-fstests/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

$(KBUILD_SCRIPTS): %: kernel-build/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@
