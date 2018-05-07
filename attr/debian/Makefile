#
# Copyright (c) 2000-2003 Silicon Graphics, Inc.  All Rights Reserved.
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
include $(TOPDIR)/include/builddefs

LIBPKG = lib$(PKG_NAME)1
LSRCFILES = changelog compat control copyright rules watch
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
