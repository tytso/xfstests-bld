#
# A simple makefile for xfstests-bld
#

REPOS =		fio \
		keyutils \
		fsverity \
		quota \
		stress-ng \
		util-linux \
		xfsprogs-dev \
		xfstests-dev \
		go/src/github.com/google/syzkaller

SUBDIRS =	acl \
		android-compat \
		attr \
		dbench \
		e2fsprogs-libs \
		libaio \
		misc \
		popt \
		$(REPOS)

SCRIPTS =	android-xfstests.sh \
		gce-xfstests.sh \
		kvm-xfstests.sh


all: $(SCRIPTS)
	./get-all
	./build-all

all-clean-first: $(SCRIPTS)
	./get-all
	rm -rf bld xfstests
	./build-all --clean-first

$(SCRIPTS): %.sh: kvm-xfstests/%.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

clean:
	for i in $(SUBDIRS) ; \
	do \
		if test -f $$i/Makefile ; then make -C $$i clean ; fi ; \
	done
	if test -d xfsprogs-dev; then make -C xfsprogs-dev realclean; fi
	rm -rf bld xfstests build-distro
	rm -f kvm-xfstests/util/zerofree $(SCRIPTS)

kvm-xfstests/util/zerofree: kvm-xfstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf $(REPOS) *.ver go

tarball:
	./gen-tarball

.PHONY: all clean realclean tarball
