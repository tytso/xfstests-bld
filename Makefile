#
# A simple makefile for xfstests-bld
#

SUBDIRS =	acl \
		android-compat \
		attr \
		e2fsprogs-libs \
		fio \
		quota \
		libaio \
		misc \
		xfsprogs-dev \
		xfstests-dev

all: xfsprogs-dev xfstests-dev fio quota kvm-xfstests/util/zerofree
	./build-all

xfsprogs-dev xfstests-dev fio quota:
	./get-all

clean:
	for i in $(SUBDIRS) ; \
	do \
		if test -f $$i/Makefile ; then make -C $$i clean ; fi ; \
	done
	make -C xfsprogs-dev realclean
	rm -rf bld xfstests
	rm -f kvm-xfstests/util/zerofree

kvm-xfstests/util/zerofree: kvm-xfstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf xfsprogs-dev xfstests-dev fio quota *.ver

tarball:
	./gen-tarball
