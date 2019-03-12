#!/bin/bash

BUILD_IMAGE=harbor.uio.no/it-usit-gid-drift/deb-build:ubuntu-latest

docker pull ${BUILD_IMAGE}

docker run -i -v "$(pwd)/pkg:/pkg:Z" -v "$(pwd):/data:Z" ${BUILD_IMAGE} <<EOF
cd /data
debuild -b -us -uc
mv ../*.deb /pkg
EOF
