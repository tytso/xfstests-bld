#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

HFILES = dmapi.h
LSRCFILES = builddefs.in buildmacros buildrules install-sh dmapi_kern.h
LDIRT = xfs

default install: xfs

xfs:
	$(LN_S) . $@

include $(BUILDRULES)

install-dev: default
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)
	$(INSTALL) -m 644 $(HFILES) $(PKG_INC_DIR)
