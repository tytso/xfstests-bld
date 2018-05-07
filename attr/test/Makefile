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

TEST = $(wildcard *.test)
EXT = $(wildcard ext/*.test)
ROOT = $(wildcard root/*.test)

# ensure we pick these up in the source tarball
LSRCFILES = $(TEST) $(EXT) $(ROOT) run sort-getfattr-output README

default install install-dev install-lib:

include $(BUILDRULES)

PATH := $(abspath ../getfattr/):$(abspath ../setfattr):$(abspath ../chattr):$(PATH)

tests: $(TEST)
ext-tests: $(EXT)
root-tests: $(ROOT)

$(TEST):
	@echo "*** $@ ***"; perl run $@

$(EXT):
	@echo "EXT specific tests"; @echo "*** $@ ***"; perl run $@

$(ROOT):
	@echo "Note: Tests must run as root"; @echo "*** $@ ***"; perl run $@

.PHONY: $(TEST) $(EXT) $(ROOT)
.NOTPARALLEL:
