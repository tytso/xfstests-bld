#
# Copyright (c) 2001-2002 Silicon Graphics, Inc.  All Rights Reserved.
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

POTHEAD = $(PKG_NAME).pot
LINGUAS ?= cs de es fr gl nl pl sv
LSRCFILES = $(LINGUAS:%=%.po) $(POTHEAD)
LDIRT = $(POTHEAD)

XGETTEXTFILES =	$(TOPDIR)/attr/attr.c \
		$(TOPDIR)/getfattr/getfattr.c \
		$(TOPDIR)/setfattr/setfattr.c \
		$(TOPDIR)/libattr/attr_copy_fd.c \
		$(TOPDIR)/libattr/attr_copy_file.c

default: $(POTHEAD) $(LINGUAS:%=%.mo)

include $(BUILDRULES)

install: default
	$(INSTALL_LINGUAS)

install-dev install-lib:
