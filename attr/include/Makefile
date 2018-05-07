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

INCDIR = attr
INST_HFILES = attributes.h xattr.h error_context.h libattr.h
HFILES = $(INST_HFILES) misc.h walk_tree.h
LSRCFILES = builddefs.in buildmacros buildrules config.h.in install-sh
LDIRT = $(INCDIR)

default: $(INCDIR)

$(INCDIR):
	$(LN_S) . $@

include $(BUILDRULES)

install-dev: default
	$(INSTALL) -m 755 -d $(PKG_INC_DIR)
	$(INSTALL) -m 644 $(INST_HFILES) $(PKG_INC_DIR)

install install-lib:
