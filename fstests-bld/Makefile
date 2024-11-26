#
# A simple makefile for xfstests-bld
#

REPOS =		blktests \
		fio \
		keyutils \
		fsverity \
		ima-evm-utils \
		libaio \
		nvme-cli \
		quota \
		stress-ng \
		util-linux \
		xfsprogs-dev \
		xfstests-dev \
		go/src/github.com/google/syzkaller

SUBDIRS =	android-compat \
		dbench \
		e2fsprogs-libs \
		libinih \
		misc \
		popt \
		$(REPOS)

SCRIPTS =	android-xfstests.sh \
		gce-xfstests.sh \
		kvm-xfstests.sh


all:
	./get-all
	./build-all $(DEBUG)

all-clean-first:
	./get-all
	rm -rf bld xfstests *.ver
	./build-all --clean-first

clean:
	for i in $(SUBDIRS) ; \
	do \
		if test -f $$i/Makefile ; then make -C $$i clean ; fi ; \
	done
	if test -d xfsprogs-dev; then make -C xfsprogs-dev realclean; fi
	rm -rf bld xfstests build-distro
	rm -f run-fstests/util/zerofree $(SCRIPTS)

run-fstests/util/zerofree: run-fstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf $(REPOS) *.ver go

tarball:
	./gen-tarball

.PHONY: all clean realclean tarball
