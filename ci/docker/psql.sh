#!/bin/bash
I="-i "
if [[ "$*" == *"-c"* ]]; then
    I=""
fi
docker run $I -t --rm --network docker_default -e "PGPASSWORD=notsecret" postgres psql -h postgres -U nivlheim nivlheim "$@"
