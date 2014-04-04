#!/bin/sh -ex
po/update-potfiles
autopoint --force
exec autoreconf -f -i
