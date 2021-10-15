#!/bin/bash

echo "------------- Testing client timing ------------"
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
    echo "The client failed to post data successfully:"
	echo "--------------------------------------------"
	cat $tempdir/output
	printlogs
    exit 1
fi

# test the "minperiod" parameter
echo 'Testing the "minperiod" parameter'
docker exec banana bash -c 'mkdir -p /var/run; touch /var/run/nivlheim_client_last_run'
set +e
docker exec banana nivlheim_client --minperiod 60 --debug >$tempdir/output 2>&1
if [[ $? -ne 64 ]]; then
	echo "The minperiod parameter for nivlheim_client had no effect."
	echo "------------- client output: -----------------"
	cat $tempdir/output
	printlogs
	exit 1
fi
docker exec banana rm -f /var/run/nivlheim_client_last_run
docker exec banana nivlheim_client --minperiod 60 --debug >$tempdir/output 2>&1
if [[ $? -eq 64 ]]; then
	echo "The minperiod option skips out even if the run file is missing."
	echo "------------- client output: -----------------"
	cat $tempdir/output
	printlogs
	exit 1
fi
set -e

# test the "sleeprandom" parameter
echo 'Testing the "sleeprandom" parameter'
docker exec banana nivlheim_client --sleeprandom 5 --debug >$tempdir/output 2>&1
if ! grep -s "sleeping" $tempdir/output; then
	echo "The sleeprandom parameter for nivlheim_client had no effect."
	printlogs
	exit 1
fi

echo "Test result: OK"
