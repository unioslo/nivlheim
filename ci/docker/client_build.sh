#!/bin/bash
cd `dirname $0`/../..  # cd to the root of the git repo
cp client/client.yaml tmp_client.yaml
echo "server=localhost" >> tmp_client.yaml
docker build -f ci/docker/client_Dockerfile --tag nivlheimclient:latest .
rm tmp_client.yaml
