#!/bin/bash
I="-it "
if [[ "$*" == *"-c"* ]]; then
    I=""
fi
docker exec $I -e PGCONNECT_TIMEOUT=30 docker_postgres_1 psql -U nivlheim -h localhost -d nivlheim "$@"
