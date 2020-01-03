#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the system database
# - The nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving CGI scripts
# - The nivlheim client is installed

echo "------------- Testing certificate handling ------------"
set -e

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Clean/init everything
sudo systemctl stop nivlheim
sudo rm -f /var/log/nivlheim/system.log /var/nivlheim/my.{crt,key} \
	/var/run/nivlheim_client_last_run /var/www/nivlheim/certs/* \
	/var/www/nivlheim/queue/*
echo -n | sudo tee /var/log/httpd/error_log
/var/nivlheim/installdb.sh --wipe
sudo systemctl start nivlheim
sleep 4

# Run the client. This will call reqcert and post
echo "Running the client"
if ! grep -s -e "^server" /etc/nivlheim/client.conf > /dev/null; then
    echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
fi
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=127.0.0.0/24'
sudo /usr/sbin/nivlheim_client
if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
    echo "The client failed to post data successfully."
    exit 1
fi

# Verify that reqcert didn't leave any files
if [[ $(ls -1 /var/www/nivlheim/certs | wc -l) -gt 0 ]]; then
	echo "Certificate files are left after reqcert"
	ls -1 /var/www/nivlheim/certs
	exit 1
fi

# Verify that the PKCS8 file was created
if [[ ! -f /var/nivlheim/pkcs8.key ]]; then
    echo "pkcs8.key is missing."
    exit 1
fi
if ! $(sudo openssl pkcs8 -in /var/nivlheim/pkcs8.key -nocrypt -out /dev/null); then
    echo "pkcs8.key is invalid."
    exit 1
fi
if [[ $(stat -c "%a" /var/nivlheim/pkcs8.key) != "600" ]]; then
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

# Stop the system daemon to prevent it from messing with the tests
sudo systemctl stop nivlheim

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

# Provoke a renewal of the cert. Do this by changing the hostname in the database.
psql -c "UPDATE hostinfo SET hostname='abcdef'"
sudo /usr/sbin/nivlheim_client
# one more time
psql -c "UPDATE hostinfo SET hostname='ghijkl'"
sudo /usr/sbin/nivlheim_client

# Verify the certificate chain
chain=$(psql --no-align -t -c "SELECT certid,first,previous FROM certificates ORDER BY certid")
expect=$(echo -e "1|1|\n2|1|1\n3|1|2\n")
if [[ "$chain" != "$expect" ]]; then
	echo "Certificate chain differs from expected value:"
	psql -c "SELECT certid,issued,first,previous,fingerprint FROM certificates ORDER BY certid"
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
