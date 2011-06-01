# foo.
TEST_SRCS:=$(shell find cases/ -name \*.t | sort -n -t/ -k2)
EXTRAPROGS:=cases/8.p cases/10.p
PARTPROGS:=$(filter-out $(EXTRAPROGS), $(patsubst %.t,%.p,$(TEST_SRCS)))
PROGS:=$(PARTPROGS) $(EXTRAPROGS)
HARNESS_SRCS:=main.c
# io_queue.c

CFLAGS=-Wall -Werror -I../src -g -O
#-lpthread -lrt

all: $(PROGS)

$(PROGS): %.p: %.t $(HARNESS_SRCS)
	$(CC) $(CFLAGS) -DTEST_NAME=\"$<\" -o $@ main.c ../src/libaio.a

clean:
	rm -f $(PROGS) *.o runtests.out rofile wofile rwfile

.PHONY:

testdir/rofile: testdir .PHONY
	rm -f $@
	echo "test" >$@
	chmod 400 $@

testdir/wofile: testdir .PHONY
	rm -f $@
	echo "test" >$@
	chmod 200 $@

testdir/rwfile: testdir .PHONY
	rm -f $@
	echo "test" >$@
	chmod 600 $@

testdir testdir.enospc testdir.ext2:
	mkdir $@

root: .PHONY
	@if [ `id -u` -ne 0 ]; then echo Need root for check, try partcheck >&2; exit 1; fi

partcheck: $(PARTPROGS) testdir/rofile testdir/rwfile testdir/wofile
	./runtests.sh $(PARTPROGS)

ext2.img:
	dd if=/dev/zero bs=1M count=10 of=$@
	mke2fs -F -b 4096 $@

extracheck: $(EXTRAPROGS) root testdir.ext2 testdir.enospc ext2.img
	mount -o loop -t ext2 ext2-enospc.img testdir.enospc
	./runtests.sh cases/10.p; ret=$$?; umount testdir.enospc; exit $$ret
	mount -o loop -t ext2 ext2.img testdir.ext2
	./runtests.sh cases/8.p; ret=$$?; umount testdir.ext2; exit $$ret

check: partcheck extracheck 
