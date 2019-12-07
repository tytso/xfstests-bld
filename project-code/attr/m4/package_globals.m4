dnl Copyright (C) 2003, 2004, 2006, 2007  Silicon Graphics, Inc.
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
# Generic macro, sets up all of the global packaging variables.
# The following environment variables may be set to override defaults:
#   DEBUG OPTIMIZER MALLOCLIB PLATFORM DISTRIBUTION INSTALL_USER INSTALL_GROUP
#   BUILD_VERSION
#
AC_DEFUN([AC_PACKAGE_GLOBALS],
  [ pkg_name="$1"
    AC_SUBST(pkg_name)

    AC_PROG_CC

    . ./VERSION
    pkg_version=${PKG_MAJOR}.${PKG_MINOR}.${PKG_REVISION}
    AC_SUBST(pkg_version)
    pkg_release=$PKG_BUILD
    test -z "$BUILD_VERSION" || pkg_release="$BUILD_VERSION"
    AC_SUBST(pkg_release)

    DEBUG=${DEBUG:-'-DDEBUG'}		dnl  -DNDEBUG
    debug_build="$DEBUG"
    AC_SUBST(debug_build)

    OPTIMIZER=${OPTIMIZER:-'-g -O2'}
    opt_build="$OPTIMIZER"
    AC_SUBST(opt_build)

    MALLOCLIB=${MALLOCLIB:-''}		dnl  /usr/lib/libefence.a
    malloc_lib="$MALLOCLIB"
    AC_SUBST(malloc_lib)

    pkg_user=`id -u -n`
    test -z "$INSTALL_USER" || pkg_user="$INSTALL_USER"
    AC_SUBST(pkg_user)

    pkg_group=`id -g -n`
    test -z "$INSTALL_GROUP" || pkg_group="$INSTALL_GROUP"
    AC_SUBST(pkg_group)

    pkg_distribution=`uname -s`
    test -z "$DISTRIBUTION" || pkg_distribution="$DISTRIBUTION"
    AC_SUBST(pkg_distribution)

    pkg_platform=`uname -s | tr 'A-Z' 'a-z' | sed -e 's/irix64/irix/'`
    test -z "$PLATFORM" || pkg_platform="$PLATFORM"
    AC_SUBST(pkg_platform)
  ])
