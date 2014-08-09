#
# A simple makefile for xfstests-bld
#

all: xfsprogs-dev xfstests-dev fio quota kvm-xfstests/util/zerofree
	./build-all

xfsprogs-dev xfstests-dev fio quota:
	./get-all

clean:
	for i in acl attr e2fsprogs-libs fio quota libaio xfstests-dev ; \
	do \
		make -C $$i clean ; \
	done
	make -C xfsprogs-dev realclean
	rm -rf bld xfstests
	rm kvm-xfstests/util/zerofree

kvm-xfstests/util/zerofree: kvm-xfstests/util/zerofree.c
	cc -static -o $@ $< -lext2fs -lcom_err -lpthread

realclean: clean
	rm -rf xfsprogs-dev xfstests-dev fio quota *.ver

tarball:
	rm -rf xfstests
	cp -r xfstests-dev xfstests
	cat *.ver > xfstests/git-versions
	rm -rf xfstests/.git xfstests/autom4te.cache
	find xfstests -type f -name \*.\[cho\]  -o -name \*.l[ao] | xargs rm
	mkdir xfstests/bin
	cp bld/sbin/* xfstests/bin
	cp bld/bin/* xfstests/bin
	-find xfstests -mindepth 2 -type f -perm +0111 | xargs strip 2> /dev/null
	tar cf - xfstests | gzip -9 > xfstests.tar.gz
