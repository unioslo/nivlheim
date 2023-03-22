#!/bin/bash
I="-i "
if [[ "$*" == *"-c"* ]]; then
    I=""
fi
docker exec $I -t docker_postgres_1 psql -U nivlheim -h localhost -d nivlheim "$@"
