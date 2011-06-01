#
# Copyright (c) 2000-2006 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = .
HAVE_BUILDDEFS = $(shell test -f $(TOPDIR)/include/builddefs && echo yes || echo no)

ifeq ($(HAVE_BUILDDEFS), yes)
include $(TOPDIR)/include/builddefs
endif

CONFIGURE = aclocal.m4 configure config.guess config.sub configure install-sh \
	    ltmain.sh m4/libtool.m4 m4/ltoptions.m4 m4/ltsugar.m4 \
	    m4/ltversion.m4 m4/lt~obsolete.m4
LSRCFILES = configure.in Makepkgs release.sh README VERSION $(CONFIGURE)

LDIRT = config.log .dep config.status config.cache confdefs.h conftest* \
	Logs/* built .census install.* install-dev.* *.gz autom4te.cache/* \
	libtool include/builddefs

LIB_SUBDIRS = include libdm
TOOL_SUBDIRS = m4 man doc debian build

SUBDIRS = $(LIB_SUBDIRS) $(TOOL_SUBDIRS)

default: include/builddefs
ifeq ($(HAVE_BUILDDEFS), no)
	$(MAKE) -C . $@
else
	$(MAKE) $(SUBDIRS)
endif

# tool/lib dependencies
# There don't appear to be any dependencies between subdirs

ifeq ($(HAVE_BUILDDEFS), yes)
include $(BUILDRULES)
else
clean:	# if configure hasn't run, nothing to clean
endif

# Recent versions of libtool require the -i option for copying auxiliary
# files (config.sub, config.guess, install-sh, ltmain.sh), while older
# versions will copy those files anyway, and don't understand -i.
LIBTOOLIZE_INSTALL = `libtoolize -n -i >/dev/null 2>/dev/null && echo -i`

configure:
	libtoolize -c $(LIBTOOLIZE_INSTALL) -f
	cp include/install-sh .
	aclocal -I m4
	autoconf

include/builddefs: configure
	./configure \
		--prefix=/ \
		--exec-prefix=/ \
		--sbindir=/bin \
		--bindir=/usr/bin \
		--libdir=/lib \
		--libexecdir=/usr/lib \
		--enable-lib64=yes \
		--includedir=/usr/include \
		--mandir=/usr/share/man \
		--datadir=/usr/share \
		$$LOCAL_CONFIGURE_OPTIONS
	touch .census

install: default $(addsuffix -install,$(SUBDIRS))
ifneq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 644 README $(PKG_DOC_DIR)
endif

install-dev: default $(addsuffix -install-dev,$(SUBDIRS))

%-install:
	$(MAKE) -C $* install

%-install-dev:
	$(MAKE) -C $* install-dev

distclean: clean
	rm -f $(LDIRT)

realclean: distclean
	rm -f $(CONFIGURE)
