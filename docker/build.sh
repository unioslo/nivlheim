#!/bin/bash
cd `dirname $0`/..  # cd to the root of the git repo
docker build -f docker/Dockerfile .
