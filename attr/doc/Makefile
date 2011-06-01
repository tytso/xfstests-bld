#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LSRCFILES = INSTALL PORTING CHANGES COPYING
LDIRT = *.gz

default: $(SUBDIRS) CHANGES.gz
	$(SUBDIRS_MAKERULE)

include $(BUILDRULES)

CHANGES.gz:
	$(ZIP) --best -c < CHANGES > $@

install: default
	$(SUBDIRS_MAKERULE)
	$(INSTALL) -m 755 -d $(PKG_DOC_DIR)
	$(INSTALL) -m 644 PORTING CHANGES.gz $(PKG_DOC_DIR)
ifeq ($(PKG_DISTRIBUTION), debian)
	$(INSTALL) -S CHANGES.gz $(PKG_DOC_DIR)/changelog.gz
else
	$(INSTALL) -m 644 COPYING $(PKG_DOC_DIR)
endif

install-dev install-lib: $(SUBDIRS)
	$(SUBDIRS_MAKERULE)
