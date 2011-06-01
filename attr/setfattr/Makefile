#
# Copyright (c) 2000, 2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LTCOMMAND = setfattr
CFILES = setfattr.c

LLDLIBS = $(LIBMISC) $(LIBATTR)
LTDEPENDENCIES = $(LIBMISC) $(LIBATTR)

default: $(LTCOMMAND)

include $(BUILDRULES)

install: default
	$(INSTALL) -m 755 -d $(PKG_BIN_DIR)
	$(LTINSTALL) -m 755 $(LTCOMMAND) $(PKG_BIN_DIR)
install-dev install-lib:
