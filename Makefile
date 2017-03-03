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
		stress-ng \
		xfsprogs-dev \
		xfstests-dev

SCRIPTS =	android-xfstests.sh \
		gce-xfstests.sh \
		kvm-xfstests.sh

all: xfsprogs-dev xfstests-dev fio quota $(SCRIPTS)
	./build-all

xfsprogs-dev xfstests-dev fio quota:
	./get-all

$(SCRIPTS): %.sh: kvm-xfstests/%.in
	sed -e "s;@DIR@;$$(pwd);" < $< > $@
	chmod +x $@

clean:
	for i in $(SUBDIRS) ; \
	do \
		if test -f $$i/Makefile ; then make -C $$i clean ; fi ; \
	done
	make -C xfsprogs-dev realclean
	rm -rf bld xfstests
	rm -f kvm-xfstests/util/zerofree $(SCRIPTS)

kvm-xfstests/util/zerofree: kvm-xfstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf xfsprogs-dev xfstests-dev fio quota *.ver

tarball:
	./gen-tarball
