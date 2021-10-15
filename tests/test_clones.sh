#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the system database
# - The nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving CGI scripts

echo "------------ Testing cloned machines ------------"
set -e
cd `dirname $0`

function printlogs() {
	echo "------- access_log -------------------------------"
	docker exec docker_nivlheimweb_1 grep -v 127.0.0.1 /var/log/httpd/access_log || true
	echo "------- error_log --------------------------------"
	docker exec docker_nivlheimweb_1 grep "cgi:error" /var/log/httpd/error_log || true
	echo "------- system.log--------------------------------"
	docker exec docker_nivlheimweb_1 cat /var/log/nivlheim/system.log || true
	echo "------- docker logs ------------------------------"
	docker logs docker_nivlheimapi_1 || true
}

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)

# create a container that's running so we can do "docker exec"
docker run --rm -d --name banana --network host --entrypoint tail nivlheimclient -f /dev/null

# ensure cleanup
function finish {
	docker rm -f banana >/dev/null 2>&1 || true
	rm -rf "$tempdir"
}
trap finish EXIT

# Whitelist the private network address ranges
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=192.168.0.0/16'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.16.0.0/12'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=10.0.0.0/8'

# Run the client. This will request a certificate too.
echo "Running the client"
if ! docker exec banana nivlheim_client --debug >$tempdir/output 2>&1; then
    echo "The client failed to post data successfully."
	echo "---------- client output ------------------------------"
	cat $tempdir/output
	printlogs
    exit 1
fi

# Copy the nonce
echo "Nonce 1 = " $(docker exec banana cat /var/nivlheim/nonce)
docker cp banana:/var/nivlheim/nonce $tempdir/noncecopy

# Run the client again, to verify that the nonce works for normal usage
if ! docker exec banana nivlheim_client --debug >$tempdir/output 2>&1; then
	echo "The client failed to post data successfully the second time."
	echo "---------- client output ----------------------------------"
	cat $tempdir/output
	printlogs
	exit 1
fi
echo "Nonce 2 = " $(docker exec banana cat /var/nivlheim/nonce)

# Pretend that I'm a clone and use the old nonce
docker cp $tempdir/noncecopy banana:/var/nivlheim/nonce
if docker exec banana nivlheim_client --debug >$tempdir/output 2>&1; then
	echo "It seems the client managed to post data with a copied nonce..."
	echo "---------- client output ----------------------------------"
	cat $tempdir/output
	printlogs
	exit 1
fi

# The certificate should be revoked now
if docker exec banana curl -skf --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key 'https://localhost/cgi-bin/secure/ping'
then
	echo "The certificate wasn't revoked!"
	printlogs
	exit 1
fi

# Check for errors
if docker exec docker_nivlheimweb_1 grep -A1 "ERROR" /var/log/nivlheim/system.log; then
	exit 1
fi
if docker logs docker_nivlheimapi_1 2>&1 | grep -i error; then
	exit 1
fi
if docker exec docker_nivlheimweb_1 grep "cgi:error" /var/log/httpd/error_log | grep -v 'random state'; then
	exit 1
fi

# Check that the database table contains 1 cert which is revoked
PSQL=../ci/docker/psql.sh
chain=$($PSQL -X --no-align -t -c "SELECT certid,revoked,first FROM certificates ORDER BY certid")
expect=$(echo -e "1|t|1\r\n")
if [[ "$chain" != "$expect" ]]; then
	echo "The certificate list differ from expectation:"
	$PSQL -e -X -c "SELECT certid,revoked,first FROM certificates ORDER BY certid"
	exit 1
fi

echo "Test result: OK"
