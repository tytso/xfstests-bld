#
# Makefile for local miscellaneous test/benchmark programs
#

PROGS= fname_benchmark

SCRIPTS= encrypt-fname-benchmark encrypt-smoketest

all: $(PROGS)

fname_benchmark: fname_benchmark.c
	cc -o fname_benchmark -O2 $<

install:
	mkdir -p $(DESTDIR)/bin
	for i in $(PROGS) $(SCRIPTS) ; do \
		cp $$i $(DESTDIR)/bin ; \
		chmod +x $(DESTDIR)/bin/$$i ; \
	done

clean:
	rm -f $(PROGS)
