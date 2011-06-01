#
# Copyright (c) 2000-2001 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LSRCFILES = INSTALL PORTING CHANGES COPYING
LDIRT = *.gz

default: CHANGES.gz

include $(BUILDRULES)

CHANGES.gz:
	$(ZIP) --best -c < CHANGES > $@

install: default
ifneq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 644 PORTING CHANGES.gz $(PKG_DOC_DIR)
	$(INSTALL) -m 644 COPYING $(PKG_DOC_DIR)
endif

LIB_DOC_DIR = $(PKG_DOC_DIR)/../libdm0
install-dev:
ifeq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -m 755 -d $(LIB_DOC_DIR)
	$(INSTALL) -m 644 PORTING CHANGES.gz $(LIB_DOC_DIR)
	$(INSTALL) -S CHANGES.gz $(LIB_DOC_DIR)/changelog.gz
endif
