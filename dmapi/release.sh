#!/bin/sh
#
# Automate generation a new release
#

. VERSION

version=${PKG_MAJOR}.${PKG_MINOR}.${PKG_REVISION}
date=`date +"%-d %B %Y"`

echo "Updating CHANGES"
sed -e "s/${version}.*/${version} (${date})/" doc/CHANGES > doc/CHANGES.tmp && \
	mv doc/CHANGES.tmp doc/CHANGES

echo "Commiting CHANGES update to git"
git-commit -a -m "${version} release"

echo "Tagging git repository"
git-tag v${version}

echo "Creating source tarball in build/tar/"
./Makepkgs

echo "Done."
echo "Please remember to push out tags using \"git push --tags\""
