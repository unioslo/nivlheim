#!/bin/bash

echo "------------ Testing which user the service runs as ------------"
set -e

if [[ `ps -U nivlheim -u nivlheim -o cmd h` != "/usr/sbin/nivlheim_service" ]]
then
	echo "The system service isn't running as the nivlheim user:"
	ps -ef | grep nivlheim
	exit 1
fi
echo "Test result: OK"
