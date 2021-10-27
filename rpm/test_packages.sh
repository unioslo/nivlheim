#!/bin/bash

# This is intended to be an end-to-end test of the server and client packages.

# Install the packages. Different methods on Fedora and CentOS.
if [ -f /etc/fedora-release ]; then
	sudo dnf copr -y enable oyvindh/Nivlheim-test
	echo "Installing Nivlheim server and client..."
	sudo dnf install -y -q nivlheim-client nivlheim-server || touch installerror
elif [ -f /etc/centos-release ]; then
	sudo yum install -y epel-release
	sudo curl -sS --retry 10 -o /etc/yum.repos.d/oyvindh-Nivlheim-test-epel-7.repo \
		https://copr.fedorainfracloud.org/coprs/oyvindh/Nivlheim-test/repo/epel-7/oyvindh-Nivlheim-test-epel-7.repo
	echo "Installing Nivlheim server and client..."
	sudo yum install -y -q nivlheim-client nivlheim-server || touch installerror
fi
if [ -f installerror ]; then
	echo "Package installation failed."
	exit 1
fi
echo "Package installation completed."

# If restorecon would change anything, it means something wasn't installed right
sudo restorecon -nvR /var/www/nivlheim /var/log/nivlheim > /tmp/changed.log
if [[ $(cat /tmp/changed.log | wc -l) -gt 0 ]]; then
	echo "restorecon indicates some files/dirs don't have the right SELinux context/type."
	echo "There could be a problem with semanage commands in setup.sh."
	exit 1
fi

# Stop Nivlheim while we mess with the database
sudo systemctl stop nivlheim

# Install and set up a local Postgres database
sudo yum install -y postgresql postgresql-server || touch installerror
if [ -f installerror ]; then
	echo "Postgres installation failed."
	exit 1
fi
if ! (sudo /usr/bin/postgresql-setup initdb || sudo /usr/bin/postgresql-setup --initdb); then
	echo "Unable to initialize PostgreSQL database. Is there an existing installation?"
	exit 1
fi

# Enable password-based authentication in Postgres, instead of the default
sudo sed -i 's/ident/md5/g' /var/lib/pgsql/data/pg_hba.conf

# Start postgres
sudo systemctl start postgresql

# Create a database and user
sudo -u postgres psql <<EOT
CREATE USER testuser;
ALTER USER testuser PASSWORD 'testpassword';
CREATE DATABASE testdb WITH OWNER testuser;
EOT
echo "Created a database and a user."

# Set the database connection parameters in server.conf
# First remove any default settings
grep -ive "^pg" /etc/nivlheim/server.conf > /tmp/server.conf \
	&& sudo mv /tmp/server.conf /etc/nivlheim/
# Then add settings
cat <<EOT | sudo tee -a /etc/nivlheim/server.conf > /dev/null
pghost=127.0.0.1
pgport=5432
pgdatabase=testdb
pguser=testuser
pgpassword=testpassword
pgsslmode=disable
EOT
echo "------- /etc/nivlheim/server.conf -------"
cat /etc/nivlheim/server.conf
echo "-----------------------------------------"

# Patch up the database
/var/nivlheim/installdb.sh

# Start nivlheim because we stopped it earlier in the script
sudo systemctl start nivlheim

# Give the system service a few seconds to start up
sleep 5

# Verify that the system service is running
if ! sudo systemctl is-active --quiet nivlheim; then
	sudo systemctl status nivlheim
	exit 1
fi

if [[ "$1" == "--installonly" ]]; then
	echo "Installed and set up Nivlheim and Postgres."
	exit 0
fi

# Verify that the API is available by direct connection
if ! curl -sSfo /dev/null http://localhost:4040/api/v2/status; then
	echo "The API is unavailable at port 4040."
	exit 1
fi

# Check that the home page is being served
if [ $(curl -sSk https://localhost/ | tee /tmp/homepage | grep -c "<title>Nivlheim</title>") -eq 0 ]; then
	echo "The web server isn't properly configured and running."
	exit 1
fi
# 3rd party libraries
for URL in $(perl -ne 'm!"(libs/.*?)"!&&print "$1\n"' < /tmp/homepage);
do
	if ! curl -sSkfo /dev/null "https://localhost/$URL"; then
		echo "The web server returns an error code for $URL"
		exit 1
	fi
done

# Check that the API is available through the main web server
if ! curl -sSkfo /dev/null https://localhost/api/v2/status; then
	echo "The API is unavailable through https."
	exit 1
fi

# Turn on debug logging
sudo sed -i.bak s/log4perl.logger.reqcert=INFO/log4perl.logger.reqcert=DEBUG/g /var/www/nivlheim/log4perl.conf
# Configure the client to use the server at localhost
echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf >/dev/null
# Run the client, it will be put on waiting list for a certificate
echo "Running the client, it will be put on waiting list for a certificate"
sudo /usr/sbin/nivlheim_client
# Approve the client, using the API
echo "Approving the client by using the API"
ID=`curl -sS 'http://localhost:4040/api/v2/manualApproval?fields=approvalId'|perl -ne 'print $1 if /"approvalId":\s+(\d+)/'`
if [[ "$ID" == "" ]]; then
	echo "Error: The client wasn't on the waiting list."
	echo "------- access_log: --------"
	sudo tail -10 /var/log/httpd/access_log
	echo "------- error_log: ---------"
	sudo tail -10 /var/log/httpd/error_log
	echo "------- system.log: --------"
	tail -10 /var/log/nivlheim/system.log
	echo "------- journal: -..--------"
	sudo journalctl -S yesterday | grep nivlheim
	exit 1
fi
curl -sSX PATCH --data "hostname=abcdef&approved=true" "http://localhost:4040/api/v2/manualApproval/$ID"

# Run the client again, this time it will receive a certificate
# and post data into the system
echo "Running the client again, this time it will get a certificate"
sudo /usr/sbin/nivlheim_client
if [ ! -f /var/nivlheim/my.crt ]; then
	echo "Certificate generation failed."
	cat /var/log/nivlheim/system.log
	sudo journalctl -S yesterday | grep nivlheim
	exit 1
fi

# wait for server to process incoming data
OK=0
for try in {1..20}; do
	sleep 3
	# Query the API for the new machine
	if [ $(curl -sS 'http://localhost:4040/api/v2/hostlist?fields=hostname' | grep -c "abcdef") -gt 0 ]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "Home page does not show the new machine."
	grep -A1 "ERROR" /var/log/nivlheim/system.log
	sudo grep "cgi:error" /var/log/httpd/error_log | grep -v 'random state'
	sudo journalctl -S yesterday -t nivlheim
	exit 1
fi

echo "Installation of packages and basic testing went well."
