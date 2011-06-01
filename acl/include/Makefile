#
# Copyright (c) 2001-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

HFILES = acl.h libacl.h acl_ea.h misc.h walk_tree.h
LSRCFILES = builddefs.in buildmacros buildrules config.h.in
LDIRT = sys acl

default:
	rm -f sys acl
	$(LN_S) . sys
	$(LN_S) . acl

include $(BUILDRULES)

install-dev: default
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)/sys
	$(INSTALL) -m 644 acl.h $(PKG_INC_DIR)/sys/acl.h
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)/acl
	$(INSTALL) -m 644 libacl.h $(PKG_INC_DIR)/acl/libacl.h
install install-lib:
