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
include $(TOPDIR)/include/builddefs

SUBDIRS = man1 man2 man3 man5

default : $(SUBDIRS)

install : $(addsuffix -install,$(SUBDIRS))

install-dev : $(addsuffix -install-dev,$(SUBDIRS))

install-lib : $(addsuffix -install-lib,$(SUBDIRS))

%-install:
	$(MAKE) -C $* install

%-install-dev:
	$(MAKE) -C $* install-dev

%-install-lib:
	$(MAKE) -C $* install-lib

include $(BUILDRULES)
