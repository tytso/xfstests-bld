#
# Copyright (c) 2001-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

INCDIR = attr
INST_HFILES = attributes.h xattr.h error_context.h libattr.h
HFILES = $(INST_HFILES) misc.h walk_tree.h
LSRCFILES = builddefs.in buildmacros buildrules config.h.in
LDIRT = $(INCDIR)

default:
	rm -f $(INCDIR)
	$(LN_S) . $(INCDIR)

include $(BUILDRULES)

install-dev: default
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)
	$(INSTALL) -m 644 $(INST_HFILES) $(PKG_INC_DIR)
install install-lib:
