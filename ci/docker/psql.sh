#!/bin/bash
I="-it "
if [[ "$*" == *"-c"* ]]; then
    I=""
fi
EXECUTABLE=docker
if ! type docker >/dev/null 2>&1; then
    EXECUTABLE=podman
fi
CONTAINER_NAME=$($EXECUTABLE ps --filter ancestor=postgres --format '{{.Names}}')
$EXECUTABLE exec $I -e PGCONNECT_TIMEOUT=30 $CONTAINER_NAME psql -U nivlheim -h localhost -d nivlheim "$@"
