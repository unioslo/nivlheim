#!/bin/bash

echo "------------- Test bootstrapping with a CFEngine certificate ------------"
set -e

# Clean/init everything
sudo systemctl stop nivlheim
sudo rm -f /var/log/nivlheim/system.log /var/nivlheim/my.{crt,key} \
	/var/run/nivlheim_client_last_run /var/www/nivlheim/certs/* \
	/var/www/nivlheim/queue/*
echo -n | sudo tee /var/log/httpd/error_log
sudo -u apache /var/nivlheim/installdb.sh --wipe

# Configure the CFengine key dir setting in the server config
if ! grep -s -e "^CFEngineKeyDir" /etc/nivlheim/server.conf > /dev/null; then
    echo "CFEngineKeyDir=/var/cfekeys" | sudo tee -a /etc/nivlheim/server.conf
fi

# Start the server process again
sudo systemctl start nivlheim
sleep 4

# Configure the server setting in the client config
if ! grep -s -e "^server" /etc/nivlheim/client.conf > /dev/null; then
    echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
fi

# Try to run the client without CFEngine signature or any form of pre-approval.
# Should result in it being put on the waiting list.
echo "Running the client without any trust"
sudo /usr/sbin/nivlheim_client --nocfe || true
A=$(sudo -u apache psql --no-align -t -c "SELECT count(*) FROM waiting_for_approval")
if [[ "$A" == "0" ]]; then
	echo "The client should have been put on the waiting list, but wasn't..."
	exit 1
fi
# Clean up
sudo -u apache psql --no-align -t -c "TRUNCATE TABLE waiting_for_approval"

# If CFEngine isn't installed on this machine, install a fake CFEngine key pair
FAKE=0
if [[ ! -d /var/cfengine ]]; then
	echo "Installing a fake CFEngine key pair for this host"
	FAKE=1
	sudo mkdir -p /var/cfengine/ppkeys
	sudo cp cfengine.priv /var/cfengine/ppkeys/localhost.priv
	sudo cp cfengine.pub /var/cfengine/ppkeys/localhost.pub
	sudo mkdir -p /var/cfekeys
	sudo cp cfengine.pub /var/cfekeys/01234567890123456789012345678932.pub   # default value for a machine without cf-key
	# Ensure the httpd process will have read access.
	# This will probably be handled differently on the actual server.
	sudo chmod -R go+r /var/cfekeys
	sudo chcon -R -t httpd_sys_content_t /var/cfekeys
fi

# Run the client. This will call reqcert and post.
# Note that no ip address ranges have been registered on the server,
# so the client will have to rely on its CFEngine key to gain trust.
echo "Running the client"
sudo /usr/sbin/nivlheim_client --debug || true

if [[ "$FAKE" == "1" ]]; then
	echo "Removing the CFEngine keys"
	sudo rm -rf /var/cfengine/ppkeys
fi

if [[ ! -f /var/run/nivlheim_client_last_run ]]; then
    echo "The client failed to post data successfully."
	cat /var/log/nivlheim/system.log
	sudo grep "cgi:error" /var/log/httpd/error_log
	sudo ausearch -i -ts recent -sv no
    exit 1
fi

# The client should have received a certificate
if [[ ! -f /var/nivlheim/my.crt ]]; then
	echo "The client didn't receive a certificate."
	exit 1
fi

# The certificate should have the field trusted_by_cfengine = true in the database
echo "Verifying the certificate in the database"
trust=$(sudo -u apache psql --no-align -t -c "SELECT trusted_by_cfengine FROM certificates")
if [[ "$trust" != "t" ]]; then
	echo "trusted_by_cfengine isn't true in the database table."
	exit 1
fi

# Wait for the files to be parsed
sleep 20

# Trigger the job that gives the machine a hostname
echo "Triggering a job so Nivlheim will assign a hostname"
curl -sSf -X POST 'http://localhost:4040/api/internal/triggerJob/handleDNSchangesJob'

# Give it time to work
sleep 5

# The host should have a hostname now
echo "Looking for the hostname in the database"
names=$(sudo -u apache psql --no-align -t -c "SELECT count(*) FROM hostinfo WHERE hostname IS NOT NULL")
if [[ "$names" -lt "1" ]]; then
	echo "The host didn't get a name in Nivlheim."
	journalctl -u nivlheim
	exit 1
fi

echo "Test result: OK"