
OBJS=ini.o
LIBDIR=$(DESTDIR)/lib
INCDIR=$(DESTDIR)/include

libinih.a: $(OBJS)
	ar rc $@ $(OBJS)
	ranlib $@

install:
	mkdir -p $(LIBDIR) $(INCDIR)
	cp ini.h $(INCDIR)
	cp libinih.a $(LIBDIR)

clean:
	rm -f ini.o libinih.a

ini.o: ini.c ini.h

