#
# Copyright (c) 2000-2003 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LIBPKG = lib$(PKG_NAME)1
LSRCFILES = changelog control copyright rules
DEV_DOC_DIR = $(PKG_DOC_DIR)/../$(LIBPKG)-dev
LIB_DOC_DIR = $(PKG_DOC_DIR)/../$(LIBPKG)

default:

include $(BUILDRULES)

install: default
ifeq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 644 changelog $(PKG_DOC_DIR)/changelog.Debian
endif

install-dev: default
ifeq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 755 -d $(DEV_DOC_DIR)
	$(INSTALL) -m 644 copyright $(DEV_DOC_DIR)
	$(INSTALL) -m 644 changelog $(DEV_DOC_DIR)/changelog.Debian
endif

install-lib: default
ifeq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 755 -d $(LIB_DOC_DIR)
	$(INSTALL) -m 644 copyright $(LIB_DOC_DIR)
	$(INSTALL) -m 644 changelog $(LIB_DOC_DIR)/changelog.Debian
endif
