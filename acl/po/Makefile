#
# Copyright (c) 2001-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

POTHEAD = $(PKG_NAME).pot
LINGUAS = de es fr gl pl sv
LSRCFILES = $(LINGUAS:%=%.po) $(POTHEAD)
LDIRT = $(POTHEAD)

XGETTEXTFILES =	$(TOPDIR)/chacl/chacl.c \
		$(TOPDIR)/setfacl/do_set.c \
		$(TOPDIR)/setfacl/setfacl.c \
		$(TOPDIR)/getfacl/getfacl.c \
		$(TOPDIR)/libacl/acl_error.c \
		$(TOPDIR)/libacl/perm_copy_fd.c \
		$(TOPDIR)/libacl/perm_copy_file.c

default: $(POTHEAD) $(LINGUAS:%=%.mo)

include $(BUILDRULES)

install: default
	$(INSTALL_LINGUAS)

install-dev install-lib:
