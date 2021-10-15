#!/bin/bash

echo "-------------- Testing creating/activating a new client CA certificate -----------"
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

# Whitelist the private network address ranges
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=192.168.0.0/16'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.16.0.0/12'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=10.0.0.0/8'

# Remove any previous volume used by the client
docker volume rm clientvar -f > /dev/null

# Run the client. This will call reqcert and post
echo "Running the client"
if ! docker run --rm --network host -v clientvar:/var nivlheimclient --debug >/tmp/output 2>&1; then
    echo "The client failed to post data successfully:"
	echo "--------------------------------------------"
	cat /tmp/output
	printlogs
    exit 1
fi

# Create a new CA certificate
echo "Attempting to create a new CA certificate..."
docker exec docker_nivlheimweb_1 /usr/bin/client_CA_cert.sh --force-create --verbose

# Start a container that has the clientvar volume mounted, for easier access
docker run -d --rm --name easyvar -v clientvar:/var --network host --entrypoint sh nivlheimclient -c 'tail -f /dev/null'
function finish {
	docker kill easyvar >/dev/null
	rm -f /tmp/output
}
trap finish EXIT

# Verify that the old client certificate still works
if ! docker exec easyvar curl -sSkf --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/ping; then
	echo "The client cert didn't work after a new CA was created."
	printlogs
	exit 1
fi

# Verify that the client doesn't ask for a new certificate yet
OLDMD5=$(docker exec easyvar md5sum /var/nivlheim/my.crt)
docker run --rm -v clientvar:/var --network host nivlheimclient
NEWMD5=$(docker exec easyvar md5sum /var/nivlheim/my.crt)
if [[ "$OLDMD5" != "$NEWMD5" ]]; then
	echo "The client got get a new certificate before the new CA was activated."
	printlogs
	exit 1
fi

# Ask for a new certificate, verify that they are still being signed with the old CA cert
A=$(docker exec easyvar openssl x509 -in /var/nivlheim/my.crt -noout -issuer_hash)
docker exec easyvar rm -f /var/nivlheim/my.* /var/run/nivlheim_client_last_run
if ! docker run --rm -v clientvar:/var --network host nivlheimclient; then
	echo "The client failed to run the second time."
	printlogs
	exit 1
fi
B=$(docker exec easyvar openssl x509 -in /var/nivlheim/my.crt -noout -issuer_hash)
if [[ "$A" != "$B" ]]; then
	echo "After creating a new CA cert, it was used for issuing even before it was activated."
	printlogs
	exit 1
fi

# Activate the new CA certificate
docker exec docker_nivlheimweb_1 /usr/bin/client_CA_cert.sh --force-activate --verbose

# Verify that the old client certificate still works
docker exec docker_nivlheimweb_1 cp -a /var/www/cgi-bin/ping /var/www/cgi-bin/secure/foo
if ! docker exec easyvar curl -sSkf --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/foo; then
	echo "The client cert didn't work after a new CA was activated."
	printlogs
	exit 1
fi

# Run the client again, verify that it asked for (and got) a new certificate
# (because secure/ping should return 400)
# and verify that it was signed with the new CA cert
OLDMD5=$(docker exec easyvar md5sum /var/nivlheim/my.crt)
docker exec easyvar rm -f /var/run/nivlheim_client_last_run
if ! docker run --rm -v clientvar:/var --network host nivlheimclient --debug >/tmp/output 2>&1; then
    echo "The client failed to run the third time."
	echo "--------------------------------------------"
	cat /tmp/output
	printlogs
    exit 1
fi
NEWMD5=$(docker exec easyvar md5sum /var/nivlheim/my.crt)
if [[ "$OLDMD5" == "$NEWMD5" ]]; then
	echo "The client didn't get a new certificate after the server got a new CA."
	printlogs
	exit 1
fi
C=`docker exec easyvar openssl x509 -in /var/nivlheim/my.crt -noout -issuer_hash`
if [[ "$B" == "$C" ]]; then
    echo "Still signing with the old CA cert, even after the new one was activated."
	printlogs
    exit 1
fi

echo "Test result: OK"
