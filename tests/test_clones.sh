#!/bin/bash

# Dependencies/assumptions:
# - It is safe and OK to make changes to the system database
# - The nivlheim system service is running
# - The API is served at localhost:4040
# - The nivlheim client is installed

echo "------------ Testing cloned machines ------------"
set -e

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Run the client
if ! grep -s -e "^server" /etc/nivlheim/client.conf; then
	echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
fi
curl -sS -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=127.0.0.0/24'
sudo /usr/sbin/nivlheim_client
curl -sS 'http://localhost:4040/api/v0/triggerJob/handleDNSchangesJob'

# Copy the nonce
cat /var/nivlheim/nonce
cp /var/nivlheim/nonce $tempdir/noncecopy

# Run the client again
sudo /usr/sbin/nivlheim_client

# Pretend that I'm a clone and use the old nonce
sudo cp $tempdir/noncecopy /var/nivlheim/nonce
sudo /usr/sbin/nivlheim_client

# The certificate should be revoked now
if sudo curl -sS --cert /var/nivlheim/my.crt --key /var/nivlheim/my.key 'https://localhost/cgi-bin/secure/ping'; then
	echo "The certificate wasn't revoked!"
	exit 1
fi

echo "OK"
