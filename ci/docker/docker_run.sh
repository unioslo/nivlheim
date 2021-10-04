#!/bin/bash
IMAGE=`docker images | head -2 | tail -1 | awk '{print $3}'`
if [ "$1" == "-it" ]; then
	ID=`docker run --rm -d -p 443:443 -v data:/var/www/nivlheim -v logs:/var/log $IMAGE`
	sleep 2  # wait for the entry point script to complete
	docker logs $ID  # show any output
	docker exec -it $ID bash
else
	docker run --rm -p 443:443 -v data:/var/www/nivlheim -v logs:/var/log $IMAGE
fi
