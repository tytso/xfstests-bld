dnl Copyright (C) 2003, 2004, 2005, 2006, 2007  Silicon Graphics, Inc.
dnl
dnl This program is free software: you can redistribute it and/or modify it
dnl under the terms of the GNU General Public License as published by
dnl the Free Software Foundation, either version 2 of the License, or
dnl (at your option) any later version.
dnl
dnl This program is distributed in the hope that it will be useful,
dnl but WITHOUT ANY WARRANTY; without even the implied warranty of
dnl MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
dnl GNU General Public License for more details.
dnl
dnl You should have received a copy of the GNU General Public License
dnl along with this program.  If not, see <http://www.gnu.org/licenses/>.
#
# Check for specified utility (env var) - if unset, fail.
#
AC_DEFUN([AC_PACKAGE_NEED_UTILITY],
  [ if test -z "$2"; then
        echo
        echo FATAL ERROR: $3 does not seem to be installed.
        echo $1 cannot be built without a working $4 installation.
        exit 1
    fi
  ])

#
# Generic macro, sets up all of the global build variables.
# The following environment variables may be set to override defaults:
#  CC MAKE LIBTOOL TAR ZIP MAKEDEPEND AWK SED ECHO SORT
#  MSGFMT MSGMERGE XGETTEXT RPM
#
AC_DEFUN([AC_PACKAGE_UTILITIES],
  [ AC_PROG_CC
    cc="$CC"
    AC_SUBST(cc)
    AC_PACKAGE_NEED_UTILITY($1, "$cc", cc, [C compiler])

    search_path="$PATH$PATH_SEPARATOR/usr/freeware/bin$PATH_SEPARATOR/bin$PATH_SEPARATOR/usr/bin$PATH_SEPARATOR/usr/local/bin"

    AC_PATH_PROGS(MAKE, gmake make,, $search_path)
    make=$MAKE
    AC_SUBST(make)
    AC_PACKAGE_NEED_UTILITY($1, "$make", make, [GNU make])

    AC_PATH_PROG(TAR, tar,, $search_path)
    tar=$TAR
    AC_SUBST(tar)

    AC_PATH_PROG(ZIP, gzip,, $search_path)
    zip=$ZIP
    AC_SUBST(zip)

    AC_PATH_PROG(MAKEDEPEND, makedepend, /bin/true)
    makedepend=$MAKEDEPEND
    AC_SUBST(makedepend)

    AC_PATH_PROG(AWK, awk,, $search_path)
    awk=$AWK
    AC_SUBST(awk)

    AC_PATH_PROG(SED, sed,, $search_path)
    sed=$SED
    AC_SUBST(sed)

    AC_PATH_PROG(ECHO, echo,, $search_path)
    echo=$ECHO
    AC_SUBST(echo)

    AC_PATH_PROG(SORT, sort,, $search_path)
    sort=$SORT
    AC_SUBST(sort)

    dnl check if symbolic links are supported
    AC_PROG_LN_S

    if test "$enable_gettext" = yes; then
        AC_PATH_PROG(MSGFMT, msgfmt,, $search_path)
        msgfmt=$MSGFMT
        AC_SUBST(msgfmt)
        AC_PACKAGE_NEED_UTILITY($1, "$msgfmt", msgfmt, gettext)

        AC_PATH_PROG(MSGMERGE, msgmerge,, $search_path)
        msgmerge=$MSGMERGE
        AC_SUBST(msgmerge)
        AC_PACKAGE_NEED_UTILITY($1, "$msgmerge", msgmerge, gettext)

        AC_PATH_PROG(XGETTEXT, xgettext,, $search_path)
        xgettext=$XGETTEXT
        AC_SUBST(xgettext)
        AC_PACKAGE_NEED_UTILITY($1, "$xgettext", xgettext, gettext)

	AC_DEFINE([ENABLE_GETTEXT], 1, [enable gettext])
    fi

    AC_PATH_PROG(RPM, rpm,, $search_path)
    rpm=$RPM
    AC_SUBST(rpm)

    dnl .. and what version is rpm
    rpm_version=0
    test -n "$RPM" && test -x "$RPM" && rpm_version=`$RPM --version \
                        | awk '{print $NF}' | awk -F. '{V=1; print $V}'`
    AC_SUBST(rpm_version)
    dnl At some point in rpm 4.0, rpm can no longer build rpms, and
    dnl rpmbuild is needed (rpmbuild may go way back; not sure)
    dnl So, if rpm version >= 4.0, look for rpmbuild.  Otherwise build w/ rpm
    if test $rpm_version -ge 4; then
        AC_PATH_PROG(RPMBUILD, rpmbuild)
        rpmbuild=$RPMBUILD
    else
        rpmbuild=$RPM
    fi
    AC_SUBST(rpmbuild)
  ])
