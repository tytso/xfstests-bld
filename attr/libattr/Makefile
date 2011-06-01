#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..

LTLDFLAGS += -Wl,--version-script,$(TOPDIR)/exports
include $(TOPDIR)/include/builddefs

LTLIBRARY = libattr.la
LT_CURRENT = 2
LT_REVISION = 0
LT_AGE = 1

CFILES = libattr.c attr_copy_fd.c attr_copy_file.c attr_copy_check.c attr_copy_action.c
HFILES = libattr.h

ifeq ($(PKG_PLATFORM),linux)
CFILES += syscalls.c
else
LSRCFILES = syscalls.c
endif

LCFLAGS = -include libattr.h

default: $(LTLIBRARY)

include $(BUILDRULES)

install:

install-lib: default
	$(INSTALL_LTLIB)

install-dev: default
	$(INSTALL_LTLIB_DEV)
