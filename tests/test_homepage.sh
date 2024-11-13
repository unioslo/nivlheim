#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the Postgres database
# - The Nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving content at localhost:443/80
# - Docker/Podman has a container image with the nivlheim client

echo "------------- Testing homepage ------------"
set -e
cd `dirname $0`  # cd to the dir where the test script is
PSQL=../ci/docker/psql.sh

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

curl -ksS https://localhost:443/ > $tempdir/homepage.html
if ! diff ../server/website/index.html $tempdir/homepage.html; then
    echo "============================================================================"
    echo "ERROR: The html that is served is different from the contents of index.html."
    echo "============================================================================"
    exit 1
fi

echo "Test result: OK"
