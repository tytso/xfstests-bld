prefix=$(HOME)

INSTALLDIR= $(DESTDIR)$(prefix)/lib/kvm-xfstests
PREBUILT_URL= https://www.kernel.org/pub/linux/kernel/people/tytso/kvm-xfstests/root_fs.img.i386
TEST=$(shell tar --exclude-ignore-recursive=.gitignore -c -f /dev/null .gitignore 2>&1)

all:
	echo "Nothing to make here; but you can use make install"

install:
	mkdir -p $(INSTALLDIR)
	if test -z "$(TEST)" ; then \
		a="--exclude-ignore-recursive=.gitignore" ; \
	fi && \
	tar -cf - $$a . | \
		tar -C $(INSTALLDIR) -xf -
	rm -f $(INSTALLDIR)/config.custom
	if test -f test-appliance/root_fs.img ; then \
		cp test-appliance/root_fs.img \
			$(INSTALLDIR)/test-appliance ; \
	else \
		curl -o $(INSTALLDIR)/test-appliance/root_fs.img \
			$(PREBUILT_URL) ; \
	fi
	mkdir -p $(DESTDIR)$(prefix)/bin
	for i in kvm-xfstests gce-xfstests ; do \
		sed -e "s;@DIR@;$(prefix)/lib;" < ../$$i.in > \
			$(DESTDIR)$(prefix)/bin/$$i ; \
		chmod +x $(DESTDIR)$(prefix)/bin/$$i ; \
	done
