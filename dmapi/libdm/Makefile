#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..
include $(TOPDIR)/include/builddefs

LTLIBRARY = libdm.la
LT_CURRENT = 0
LT_REVISION = 4
LT_AGE = 0

HFILES = dmapi_lib.h
CFILES = dmapi_lib.c dm_attr.c dm_bulkattr.c dm_config.c dm_dmattr.c \
	 dm_event.c dm_handle.c dm_handle2path.c dm_hole.c dm_mountinfo.c \
	 dm_region.c dm_right.c dm_rdwr.c dm_session.c

ifeq ($(PKG_PLATFORM),linux)
HFILES += getdents.h
CFILES += getdents.c
else
LSRCFILES = getdents.h getdents.c
endif

default: $(LTLIBRARY)

include $(BUILDRULES)

install: default
	$(INSTALL_LTLIB)

install-dev: default
	$(INSTALL_LTLIB_DEV)
