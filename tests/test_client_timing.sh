#!/bin/bash

echo "------------- Testing client timing ------------"
set -e

if [[ "$1" != "--skipsetup" ]]; then
	# Clean/init everything
	sudo systemctl stop nivlheim
	sudo rm -f /var/log/nivlheim/system.log /var/nivlheim/my.{crt,key} \
		/var/run/nivlheim_client_last_run /var/www/nivlheim/certs/*
	echo -n | sudo tee /var/log/httpd/error_log
	sudo -u apache bash -c "psql -q -X -1 -v ON_ERROR_STOP=1 -f /var/nivlheim/init.sql"
	sudo systemctl start nivlheim
	sleep 4

	# Run the client. This will request a certificate too.
	if ! grep -s -e "^server" /etc/nivlheim/client.conf > /dev/null; then
	    echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
	fi
	curl -sS -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=127.0.0.0/24'
	sudo /usr/sbin/nivlheim_client
	if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
	    echo "The client failed to post data successfully."
	    exit 1
	fi
fi

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# test the "minperiod" parameter
sudo touch /var/run/nivlheim_client_last_run
set +e
sudo /usr/sbin/nivlheim_client --minperiod 60
if [[ $? -ne 64 ]]; then
	echo "The minperiod parameter for nivlheim_client had no effect."
	exit 1
fi
set -e

# test the "sleep" parameter
sudo /usr/sbin/nivlheim_client --sleeprandom 5 --debug > $tempdir/output
if ! grep -s "sleeping" $tempdir/output; then
	echo "The sleeprandom parameter for nivlheim_client had no effect."
	exit 1
fi

echo "Test result: OK"
