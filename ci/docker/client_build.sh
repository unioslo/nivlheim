#!/bin/bash
cd `dirname $0`/../..  # cd to the root of the git repo
cp client/client.conf tmp_client.conf
echo "server=localhost" >> tmp_client.conf
docker build -f ci/docker/client_Dockerfile --tag nivlheimclient:latest .
rm tmp_client.conf
