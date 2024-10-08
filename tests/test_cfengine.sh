#!/bin/bash

echo "------------- Test bootstrapping with a CFEngine certificate ------------"
set -e
cd `dirname $0`
PSQL=../ci/docker/psql.sh

# Configure where reqcert will look for CFEngine keys
docker exec docker-nivlheimweb-1 sh -c 'echo "CFEngineKeyDir=/var/cfekeys" >> /etc/nivlheim/server.conf'

# Try to run the client without CFEngine signature or any form of pre-approval.
# Should result in it being put on the waiting list.
echo "Running the client without any trust"
OUTPUT=$(docker run --rm --network host -v clientvar:/var nivlheimclient --nocfe --debug 2>&1) || true
A=$($PSQL --no-align -t -c "SELECT count(*) FROM waiting_for_approval")
if [[ "$A" == "0" ]]; then
	echo "The client should have been put on the waiting list, but wasn't..."
	echo $OUTPUT
	exit 1
fi
# Clean up
$PSQL -X --no-align -t -q -c "TRUNCATE TABLE waiting_for_approval"

# Install a fake CFEngine key pair on a client container
docker run --rm -v clientvar:/var --entrypoint sh nivlheimclient -c 'mkdir -p /var/cfengine/ppkeys'
docker create --name banana --network host -v clientvar:/var nivlheimclient --debug
function finish {
	docker rm -f banana >/dev/null 2>&1 || true
}
trap finish EXIT
docker cp cfengine.priv banana:/var/cfengine/ppkeys/localhost.priv
docker cp cfengine.pub banana:/var/cfengine/ppkeys/localhost.pub
# and the public key will also be used by the server
docker exec docker-nivlheimweb-1 mkdir -p /var/cfekeys
docker cp cfengine.pub docker-nivlheimweb-1:/var/cfekeys/root-MD5=01234567890123456789012345678932.pub   # default value for a machine without cf-key
# Ensure the httpd process will have read access
docker exec docker-nivlheimweb-1 chmod -R go+r /var/cfekeys

function printlogs() {
	echo "------- access_log -------------------------------"
	docker exec docker-nivlheimweb-1 grep -v 127.0.0.1 /var/log/httpd/access_log || true
	echo "------- error_log --------------------------------"
	docker exec docker-nivlheimweb-1 grep "cgi:error" /var/log/httpd/error_log || true
	echo "------- system.log--------------------------------"
	docker exec docker-nivlheimweb-1 cat /var/log/nivlheim/system.log || true
	echo "------- docker logs ------------------------------"
	docker logs docker-nivlheimapi-1 || true
}

# Run the client. This will call reqcert and post.
# Note that no ip address ranges have been registered on the server,
# so the client will have to rely on its CFEngine key to gain trust.
echo "Running the client, using its CFEngine key this time"
if ! docker start -a banana >/tmp/output 2>&1; then
	echo "The client failed to post data successfully."
	echo "---------- client output ----------------------"
	cat /tmp/output
	printlogs
	exit 1
fi
if ! grep -q "CFEngine signature" /tmp/output; then
	echo "The client did not use its CFEngine key."
	echo "---------- client output ----------------------"
	cat /tmp/output
	printlogs
	exit 1
fi
rm -f /tmp/output

# The client should have received a certificate
rm -f /tmp/my.crt
docker cp banana:/var/nivlheim/my.crt /tmp
if [[ ! -f /tmp/my.crt ]]; then
	echo "The client didn't receive a certificate."
	printlogs
	exit 1
fi
rm /tmp/my.crt

# The certificate should have the field trusted_by_cfengine = true in the database
echo "Verifying the certificate in the database"
trust=$($PSQL --no-align -t -c "SELECT trusted_by_cfengine FROM certificates" | tr -d '\r\n')
if [[ "$trust" != "t" ]]; then
	echo "trusted_by_cfengine isn't true in the database table."
	printlogs
	exit 1
fi

# Wait for the files to be parsed
echo "Waiting for the files to be parsed"
OK=0
for try in {1..20}; do
	sleep 3
	echo -n "."
	count=$($PSQL --no-align -t -c "SELECT count(*) FROM files WHERE parsed" | tr -d '\r\n')
	if [[ "$count" -gt "0" ]]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The files were never parsed."
	$PSQL -c "select filename, length(content), parsed, os_hostname from files"
	$PSQL -c "select * from tasks"
	exit 1
fi
echo

# Trigger the job that gives the machine a hostname
echo "Triggering a job so Nivlheim will assign a hostname"
curl -sSf -X POST 'http://localhost:4040/api/internal/triggerJob/handleDNSchangesJob'

# The host should get a hostname now
echo "Looking for the hostname in the database"
OK=0
for try in {1..40}; do
	sleep 3
	echo -n "."
	names=$($PSQL --no-align -t -c "SELECT count(*) FROM hostinfo WHERE hostname IS NOT NULL" | tr -d '\r\n')
	if [[ "$names" -ge "1" ]]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The host didn't get a name in Nivlheim."
	printlogs
	exit 1
fi
echo

echo "Test result: OK"
