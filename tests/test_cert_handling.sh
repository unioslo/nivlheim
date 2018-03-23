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
	/var/run/nivlheim_client_last_run /var/www/nivlheim/certs/*
sudo -u apache bash -c "psql -q -X -1 -v ON_ERROR_STOP=1 -f /var/nivlheim/init.sql"
sudo systemctl start nivlheim

# Run the client. This will call reqcert and post
if ! grep -s -e "^server" /etc/nivlheim/client.conf; then
    echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
fi
curl -sS -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=127.0.0.0/24'
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

# wait until the machine shows up in hostinfo
echo "Waiting for the machine to show up in hostinfo"
OK=0
for try in {1..20}; do
	sleep 3
	echo -n "."
	# Query the API for the new machine
	if [ $(curl -sS 'http://localhost:4040/api/v0/hostlist?fields=hostname' | grep -c "hostname") -gt 0 ]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The machine never showed up in hostinfo."
	exit 1
fi
echo ""

# Provoke a renewal of the cert. Do this by changing the hostname in the database.
sudo psql apache -c "UPDATE hostinfo SET hostname='abcdef'"
sudo /usr/sbin/nivlheim_client
# one more time
sudo psql apache -c "UPDATE hostinfo SET hostname='ghijkl'"
sudo /usr/sbin/nivlheim_client

# Verify the certificate chain
chain=$(sudo psql apache --no-align -t -c "SELECT certid,first,previous FROM certificates ORDER BY certid")
expect=$(echo -e "1|1|\n2|1|1\n3|1|2\n")
if [[ "$chain" != "$expect" ]]; then
	echo "Certificate chain differs from expected value:"
	sudo psql apache -c "SELECT certid,issued,first,previous,fingerprint FROM certificates ORDER BY certid"
	exit 1
fi

# Verify that renewcert didn't leave any files
if [[ $(ls -1 /var/www/nivlheim/certs | wc -l) -gt 0 ]]; then
	echo "Certificate files are left after renewcert"
	ls -1 /var/www/nivlheim/certs
	exit 1
fi

# Blacklist and check response
sudo psql apache -q -c "UPDATE certificates SET revoked=true"
# Test ping
if sudo curl -sf --cacert /var/www/nivlheim/CA/nivlheimca.crt \
	--cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/ping; then
	echo "Secure/ping worked even though cert was blacklisted."
	exit 1
fi
# Test post (it will get a 403 anyway, because the nonce is missing)
sudo curl -sS --cacert /var/www/nivlheim/CA/nivlheimca.crt \
	--cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/post > $tempdir/postresult
if ! grep -qi "revoked" $tempdir/postresult; then
	echo "Post worked even though cert was blacklisted."
	exit 1
fi
# Test renew
sudo curl -sf --cacert /var/www/nivlheim/CA/nivlheimca.crt \
	--cert /var/nivlheim/my.crt --key /var/nivlheim/my.key \
	https://localhost/cgi-bin/secure/renewcert > $tempdir/renewresult
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

echo "Test result: OK"
