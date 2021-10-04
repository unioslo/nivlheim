#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the Postgres database
# - The Nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving CGI scripts at localhost:443/80
# - Docker has a container image with the nivlheim client

echo "------------- Testing certificate handling ------------"
set -e

# Put a marker in the httpd access log
curl -sSkf 'https://localhost/====_Testing_certificate_handling_====' 2>/dev/null || true

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Whitelist the Docker network address range
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.0.0.0/8'

# Remove any previous volume used by the client
docker volume rm clientvar -f > /dev/null

# Run the client. This will call reqcert and post
echo "Running the client"
if ! docker run --rm --network host -v clientvar:/var nivlheimclient; then
    echo "The client failed to post data successfully."
    exit 1
fi

# Verify that reqcert didn't leave any files
OUTPUT=$(docker exec -it docker_web_1 ls -1 /var/www/nivlheim/certs)
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

echo "This is the end of the line"
exit

# Read database connection options from server.conf and set ENV vars for psql
if [[ -r "/etc/nivlheim/server.conf" ]]; then
	# grep out the postgres config options and make the names upper case
	grep -ie "^pg" /etc/nivlheim/server.conf | sed -e 's/\(.*\)=/\U\1=/' > /tmp/dbconf
	source /tmp/dbconf
	rm /tmp/dbconf
else
	echo "Unable to read server.conf"
	exit 1
fi
export PGHOST PGPORT PGDATABASE PGUSER PGPASSWORD

# Let's see what's in hostinfo
psql -c "SELECT hostname,certfp FROM hostinfo"

# Provoke a renewal of the cert. Do this by changing the hostname in the database.
psql -c "UPDATE hostinfo SET hostname='abcdef'"
sudo /usr/sbin/nivlheim_client --debug > /tmp/first 2>&1
# one more time
sleep 3
psql -c "UPDATE hostinfo SET hostname='ghijkl'"
sudo /usr/sbin/nivlheim_client --debug > /tmp/second 2>&1

# Verify the certificate chain
chain=$(psql --no-align -t -c "SELECT certid,first,previous FROM certificates ORDER BY certid")
expect=$(echo -e "1|1|\n2|1|1\n3|1|2\n")
if [[ "$chain" != "$expect" ]]; then
	echo "Certificate chain differs from expected value:"
	psql -c "SELECT certid,issued,first,previous,fingerprint FROM certificates ORDER BY certid"
	echo "================= httpd log:  ========================="
	sudo tail -20 /var/log/httpd/access_log
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
	echo ""
	echo "journal:"
	journalctl -u nivlheim
	exit 1
fi

# Verify that renewcert didn't leave any files
if [[ $(ls -1 /var/www/nivlheim/certs | wc -l) -gt 0 ]]; then
	echo "Certificate files are left after renewcert"
	ls -1 /var/www/nivlheim/certs
	exit 1
fi

# Set a password on the key file and verify that the client is able to handle it
pushd /var/nivlheim
sudo openssl rsa -in my.key -out my.encrypted.key -outform PEM -passout pass:passord123 -des3
sudo mv my.key my.old.key && sudo mv my.encrypted.key my.key
sudo rm -f /var/run/nivlheim_client_last_run
sudo /usr/sbin/nivlheim_client
if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
    echo "The client failed to use a password-protected certificate key."
    exit 1
fi
sudo mv -f my.old.key my.key # restore the old key file
popd

# Blacklist and check response
psql -q -c "UPDATE certificates SET revoked=true"
# Test ping
if sudo curl -skf --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/ping; then
	echo "Secure/ping worked even though cert was blacklisted."
	exit 1
fi
# Test post (it will get a 403 anyway, because the nonce is missing)
sudo curl -sk --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/post > $tempdir/postresult || true
if ! grep -qi "revoked" $tempdir/postresult; then
	echo "Post worked even though cert was blacklisted."
	exit 1
fi
# Test renew
sudo curl -sk --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/renewcert > $tempdir/renewresult || true
if ! grep -qi "revoked" $tempdir/renewresult; then
	echo "Renewcert worked even though cert was blacklisted."
	exit 1
fi

# Check logs for errors
if grep -A1 "ERROR" /var/log/nivlheim/system.log; then
    exit 1
fi
if journalctl -u nivlheim | grep -i error; then
    exit 1
fi
if sudo grep "cgi:error" /var/log/httpd/error_log | grep -v 'random state'; then
    exit 1
fi

echo "Test result: OK"
