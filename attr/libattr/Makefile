#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
# Copyright (C) 2009  Andreas Gruenbacher <agruen@suse.de>
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 2 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
