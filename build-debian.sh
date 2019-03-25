#!/bin/bash

# should use pristine-tar here
ARCHIVE=nivlheim_1.9.3.orig.tar.gz
rm -f ${ARCHIVE}
tar -zcf ${ARCHIVE} * .git

mkdir build
cd build
tar -zvxf ../${ARCHIVE}
debuild -us -uc
cd ..
rm -rf build
