#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the system database
# - The nivlheim system service is running
# - The API is served at localhost:4040
# - The web server is running and serving CGI scripts
# - The nivlheim client is installed

echo "------------ Testing cloned machines ------------"
set -e

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Clean/init everything
sudo systemctl stop nivlheim
sudo rm -f /var/log/nivlheim/system.log /var/nivlheim/my.{crt,key} /var/run/nivlheim_client_last_run
echo -n | sudo tee /var/log/httpd/error_log
sudo -u apache bash -c "psql -q -X -1 -v ON_ERROR_STOP=1 -f /var/nivlheim/init.sql"
sudo systemctl start nivlheim
sleep 4

# Run the client
if ! grep -s -e "^server" /etc/nivlheim/client.conf; then
	echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
fi
curl -sS -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=127.0.0.0/24'
sudo /usr/sbin/nivlheim_client
if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
	echo "The client failed to post data successfully."
	exit 1
fi

# Copy the nonce
echo "Nonce = " $(sudo cat /var/nivlheim/nonce)
sudo cp /var/nivlheim/nonce $tempdir/noncecopy

# Run the client again, to verify that the nonce works for normal usage
sudo rm -f /var/run/nivlheim_client_last_run
sudo /usr/sbin/nivlheim_client
if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
	echo "The client failed to post data successfully the second time."
	exit 1
fi

# Pretend that I'm a clone and use the old nonce
sudo cp $tempdir/noncecopy /var/nivlheim/nonce
sudo rm -f /var/run/nivlheim_client_last_run
sudo /usr/sbin/nivlheim_client
if [[ -f /var/run/nivlheim_client_last_run ]]; then
	echo "It seems the client managed to post data with a copied nonce..."
	exit 1
fi

# The certificate should be revoked now
if sudo curl -sf --cacert /var/www/nivlheim/CA/nivlheimca.crt \
	--cert /var/nivlheim/my.crt --key /var/nivlheim/my.key 'https://localhost/cgi-bin/secure/ping'
then
	echo "The certificate wasn't revoked!"
	exit 1
fi

# Check for errors
if grep -A1 "ERROR" /var/log/nivlheim/system.log; then
	exit 1
fi
if journalctl -u nivlheim | grep -i error; then
	exit 1
fi
if sudo grep "cgi:error" /var/log/httpd/error_log | grep -v 'random state'; then
	exit 1
fi

# Check that the database table contains 2 certs, 1 revoked
chain=$(sudo psql apache --no-align -t -c "SELECT certid,revoked,first FROM certificates ORDER BY certid")
expect=$(echo -e "1|t|1\n")
if [[ "$chain" != "$expect" ]]; then
	sudo psql apache -c "SELECT certid,revoked,first FROM certificates ORDER BY certid"
	echo "The certificate list differ from expectation"
	exit 1
fi

echo "Test result: OK"
