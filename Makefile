SCRIPTS =	android-xfstests \
		gce-xfstests \
		kvm-xfstests

KBUILD_SCRIPTS = kbuild install-kconfig

all: $(SCRIPTS) $(KBUILD_SCRIPTS)

clean:
	rm -f $(SCRIPTS) $(KBUILD_SCRIPTS)

$(SCRIPTS): %: run-fstests/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

$(KBUILD_SCRIPTS): %: kernel-build/%.sh.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@
