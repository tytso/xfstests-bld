#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

SUBDIRS = man1 man2 man3 man5

default install install-dev install-lib: $(SUBDIRS)
	$(SUBDIRS_MAKERULE)

include $(BUILDRULES)
