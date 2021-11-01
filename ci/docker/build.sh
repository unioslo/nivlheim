#!/bin/bash
cd `dirname $0`/../..  # cd to the root of the git repo
docker build -f ci/docker/Dockerfile --tag nivlheim-www:latest .
