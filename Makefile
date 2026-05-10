SCRIPTS =	android-xfstests \
		gce-xfstests \
		kvm-xfstests \
		qemu-xfstests

KBUILD_SCRIPTS = kbuild kbuild32 install-kconfig

bindir= $(HOME)/bin
completiondir= $(HOME)/.local/share/bash-completion/completions

all: $(SCRIPTS) $(KBUILD_SCRIPTS)

clean:
	rm -f $(SCRIPTS) $(KBUILD_SCRIPTS)

install: $(SCRIPTS) $(KBUILD_SCRIPTS)
	mkdir -p $(DESTDIR)$(bindir)
	for i in $(SCRIPTS) $(KBUILD_SCRIPTS) ; do \
		rm -f $(DESTDIR)$(bindir)/$$i ; \
		install $$i $(DESTDIR)$(bindir)/$$i; \
	done
	mkdir -p $(DESTDIR)/$(completiondir)
	ln -sf "$$(pwd)/run-fstests/bash-completion" $(DESTDIR)/$(completiondir)/_xfstests-bld
	for i in $(SCRIPTS) $(KBUILD_SCRIPTS) ; do \
		ln -sf _xfstests-bld $(DESTDIR)/$(completiondir)/$$i ; \
	done


$(SCRIPTS): %: run-fstests/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

$(KBUILD_SCRIPTS): %: kernel-build/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@
