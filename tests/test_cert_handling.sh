#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the Postgres database
# - The Nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving CGI scripts at localhost:443/80
# - Docker has a container image with the nivlheim client

echo "------------- Testing certificate handling ------------"
set -e
cd `dirname $0`  # cd to the dir where the test script is

# Put a marker in the httpd access log
curl -sSkf 'https://localhost/====_Testing_certificate_handling_====' 2>/dev/null || true

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Whitelist the private network address ranges
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=192.168.0.0/16'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.16.0.0/12'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=10.0.0.0/8'

# Remove any previous volume used by the client
docker volume rm clientvar -f > /dev/null

# Remove any previous cert files on the server
docker exec docker_nivlheimweb_1 sh -c 'rm -f /var/www/nivlheim/certs/*'

# Run the client. This will call reqcert and post
echo "Running the client"
if ! docker run --rm --network host -v clientvar:/var nivlheimclient --debug >/tmp/output 2>&1; then
    echo "The client failed to post data successfully:"
	echo "--------------------------------------------"
	cat /tmp/output
	echo "access_log: --------------------------------"
	docker exec docker_nivlheimweb_1 cat /var/log/httpd/access_log
	echo "error_log: ---------------------------------"
	docker exec docker_nivlheimweb_1 cat /var/log/httpd/error_log
    exit 1
fi

# Verify that reqcert didn't leave any files
OUTPUT=$(docker exec -t docker_nivlheimweb_1 ls -1 /var/www/nivlheim/certs)
if [[ "$OUTPUT" != "" ]]; then
	echo "Certificate files are left after reqcert:"
	echo $OUTPUT
	exit 1
fi

# Verify that the PKCS8 file was created
if ! docker run --rm  --entrypoint ls -v clientvar:/var nivlheimclient /var/nivlheim/pkcs8.key >/dev/null; then
    echo "pkcs8.key is missing."
    exit 1
fi
if ! docker run --rm  --entrypoint openssl -v clientvar:/var nivlheimclient pkcs8 -in /var/nivlheim/pkcs8.key -nocrypt -out /dev/null; then
    echo "pkcs8.key is invalid."
    exit 1
fi
if [[ $(docker run --rm  --entrypoint stat -v clientvar:/var nivlheimclient -c "%a" /var/nivlheim/pkcs8.key) != "600" ]]; then
	echo "pkcs8.key should have permissions 600"
	exit 1
fi

# wait until the machine shows up in hostinfo
echo "Waiting for the machine to show up in hostinfo"
OK=0
for try in {1..20}; do
	sleep 3
	echo -n "."
	# Query the API for the new machine
	if [ $(curl -sS 'http://localhost:4040/api/v2/hostlist?fields=hostname' | grep -c "hostname") -gt 0 ]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The machine never showed up in hostinfo."
	exit 1
fi
echo ""

# Let's see what's in hostinfo
PSQL=../ci/docker/psql.sh
$PSQL -e -c "SELECT hostname,certfp FROM hostinfo"

# Provoke a renewal of the cert. Do this by changing the hostname in the database.
$PSQL -c "UPDATE hostinfo SET hostname='abcdef'"
docker run --rm --network host -v clientvar:/var nivlheimclient --debug > /tmp/first 2>&1
# one more time
sleep 3
$PSQL -c "UPDATE hostinfo SET hostname='ghijkl'"
docker run --rm --network host -v clientvar:/var nivlheimclient --debug > /tmp/second 2>&1

# Verify the certificate chain
chain=$($PSQL --no-align -t -c "SELECT certid,first,previous FROM certificates ORDER BY certid")
expect=$(echo -e "1|1|\r\n2|1|1\r\n3|1|2\r\n")
if [[ "$chain" != "$expect" ]]; then
	echo "Certificate chain differs from expected value:"
	echo "$chain"
	echo "Details:"
	$PSQL -c "SELECT certid,issued,first,previous,fingerprint FROM certificates ORDER BY certid"
	echo "================= httpd access log:  =================="
	docker exec docker_nivlheimweb_1 tail -20 /var/log/httpd/access_log
	echo "================= client output (1st time): ==========="
	cat /tmp/first
	echo "================= client output (2nd time): ==========="
	cat /tmp/second
	exit 1
fi

# Verify that the GREP api returns data with the new hostname (regression test; had a bug earlier)
curl -sS 'http://localhost:4040/api/v2/grep?q=linux' > $tempdir/grepout
if ! grep -q 'ghijkl' $tempdir/grepout; then
	echo "The grep API returned unexpected results:"
	cat $tempdir/grepout
	$PSQL -e -c "SELECT hostname,certfp FROM hostinfo"
	#echo ""
	#echo "journal:"
	#journalctl -u nivlheim
	exit 1
fi

# Verify that renewcert didn't leave any files
OUTPUT=$(docker exec -t docker_nivlheimweb_1 ls -1 /var/www/nivlheim/certs)
if [[ "$OUTPUT" != "" ]]; then
	echo "Certificate files are left after renewcert:"
	echo $OUTPUT
	exit 1
fi

# Blacklist and check response
$PSQL -q -c "UPDATE certificates SET revoked=true"
# Test ping
if docker run --rm -v clientvar:/var --network host --entrypoint curl nivlheimclient -skf --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/ping; then
	echo "Secure/ping worked even though cert was blacklisted."
	exit 1
fi
# Test post (it will get a 403 anyway, because the nonce is missing)
docker run --rm -v clientvar:/var --network host --entrypoint curl nivlheimclient -sk --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/post > $tempdir/postresult || true
if ! grep -qi "revoked" $tempdir/postresult; then
	echo "Post worked even though cert was blacklisted."
	exit 1
fi
# Test renew
docker run --rm -v clientvar:/var --network host --entrypoint curl nivlheimclient -sk --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/renewcert > $tempdir/renewresult || true
if ! grep -qi "revoked" $tempdir/renewresult; then
	echo "Renewcert worked even though cert was blacklisted."
	exit 1
fi

# Check logs for errors
if docker exec -t docker_nivlheimweb_1 grep -A1 "ERROR" /var/log/nivlheim/system.log; then
    exit 1
fi
if docker logs docker_nivlheimapi_1 2>&1 | grep -i error; then
    exit 1
fi
if docker exec -t docker_nivlheimweb_1 grep "cgi:error" /var/log/httpd/error_log | grep -v 'random state'; then
    exit 1
fi

echo "Test result: OK"
