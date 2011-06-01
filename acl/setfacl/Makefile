#
# Copyright (c) 2000, 2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LTCOMMAND = setfacl
CFILES = setfacl.c do_set.c sequence.c parse.c
HFILES = sequence.h parse.h

LLDLIBS = $(LIBMISC) $(LIBACL) $(LIBATTR)
LTDEPENDENCIES = $(LIBMISC) $(LIBACL)

default: $(LTCOMMAND)

include $(BUILDRULES)

install: default
	$(INSTALL) -m 755 -d $(PKG_BIN_DIR)
	$(LTINSTALL) -m 755 $(LTCOMMAND) $(PKG_BIN_DIR)
install-dev install-lib:
