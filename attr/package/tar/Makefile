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

TOPDIR = ../..
include $(TOPDIR)/include/builddefs

BINTAR=$(PKG_NAME)-$(PKG_VERSION).tar.gz
LDIRT = *.gz

default install install-dev install-lib:

include $(BUILDRULES)

dist : default
	@HERE=`pwd`; cd $${DIST_ROOT:-/}; \
	$(SORT) -u $$HERE/../bin-manifest | $(AWK) ' \
		$$1 == "f" { printf (".%s\n", $$6); } \
		$$1 == "d" { next; } \
		$$1 == "l" { printf (".%s\n", $$3); }' \
	| $(TAR) -T - -cf - | $(ZIP) --best > $$HERE/$(BINTAR)
	@echo Wrote: `pwd`/$(BINTAR)
