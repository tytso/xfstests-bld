#
# A simple makefile for xfstests-bld
#

SUBDIRS =	acl \
		android-compat \
		attr \
		dbench \
		e2fsprogs-libs \
		fio \
		quota \
		libaio \
		misc \
		popt \
		xfsprogs-dev \
		xfstests-dev

all: xfsprogs-dev xfstests-dev fio quota \
	gce-xfstests.sh kvm-xfstests.sh
	./build-all

xfsprogs-dev xfstests-dev fio quota:
	./get-all

gce-xfstests.sh: gce-xfstests.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

kvm-xfstests.sh: kvm-xfstests.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

clean:
	for i in $(SUBDIRS) ; \
	do \
		if test -f $$i/Makefile ; then make -C $$i clean ; fi ; \
	done
	make -C xfsprogs-dev realclean
	rm -rf bld xfstests
	rm -f kvm-xfstests/util/zerofree gce-xfstests.sh kvm-xfstests.sh

kvm-xfstests/util/zerofree: kvm-xfstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf xfsprogs-dev xfstests-dev fio quota *.ver

tarball:
	./gen-tarball
