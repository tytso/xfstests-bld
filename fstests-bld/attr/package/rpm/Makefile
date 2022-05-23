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
TREEROOT = $(shell cd ${TOPDIR}; pwd)
include $(TOPDIR)/include/builddefs

SPECF = $(PKG_NAME).spec
LDIRT = *.rpm $(SPECF) rpmmacros rpmfiles* rpm-*.rc

LSRCFILES = macros.template $(SPECF).in rpm-2.rc.template

default install install-dev install-lib:

include $(BUILDRULES)

# Generate a binary rpm file
dist : default $(SPECF) rpm-$(RPM_VERSION).rc
	$(RPMBUILD) -ba --rcfile ./rpm-$(RPM_VERSION).rc $(SPECF)

# Because rpm prior to v.2.90 does not support macros and old style config
# is not supported by rpm v.3, we have to resort to such ugly hacks
ifneq ($(RPM_VERSION),2)
rpm-$(RPM_VERSION).rc : rpmmacros
	@$(SED) -e '/^macrofiles:/s|~/.rpmmacros|rpmmacros|' </usr/lib/rpm/rpmrc >$@

rpmmacros : macros.template
	@$(SED) -e 's|%topdir%|$(TREEROOT)|g' < $< > $@
else
rpm-2.rc: rpm-2.rc.template
	@$(SED) -e 's|%topdir%|$(TOPDIR)|g' < $< > $@
endif

# Generate the rpm specfile format file list from the install-sh manifest
rpmfiles rpmfiles-dev rpmfiles-lib:
	$(SORT) -u $$DIST_MANIFEST | $(AWK) > $@ '\
$$1 == "d" { printf ("%%%%dir %%%%attr(%s,%s,%s) %s\n", $$2, $$3, $$4, $$5); } \
$$1 == "f" { if (match ($$6, "$(PKG_MAN_DIR)") || \
                match ($$6, "$(PKG_DOC_DIR)")) \
                printf ("%%%%doc "); \
            if (match ($$6, "$(PKG_MAN_DIR)")) \
                printf ("%%%%attr(%s,%s,%s) %s*\n", $$2, $$3, $$4, $$6); \
            else \
                printf ("%%%%attr(%s,%s,%s) %s\n", $$2, $$3, $$4, $$6); } \
$$1 == "l" { if (match ($$3, "$(PKG_MAN_DIR)") || \
                match ($$3, "$(PKG_DOC_DIR)")) \
                printf ("%%%%doc "); \
            if (match ($$3, "$(PKG_MAN_DIR)")) \
                printf ("%%%%attr(0777,root,root) %s*\n", $$3); \
            else \
                printf ("%%%%attr(0777,root,root) %s\n", $$3); }'

.PHONY: $(SPECF)
${SPECF} : ${SPECF}.in
	$(SED) -e's|@pkg_name@|$(PKG_NAME)|g' \
	    -e's|@pkg_version@|$(PKG_VERSION)|g' \
	    -e's|@pkg_release@|$(PKG_RELEASE)|g' \
	    -e's|@pkg_distribution@|$(PKG_DISTRIBUTION)|g' \
	    -e's|@build_root@|$(DIST_ROOT)|g' \
	    -e'/^BuildRoot: *$$/d' \
	    -e's|@make@|$(MAKE)|g' < $< > $@
