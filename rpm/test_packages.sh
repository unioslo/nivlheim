#!/bin/bash
set -x

# This is intended to be an end-to-end test of the server and client packages.
# It should signal success by outputting "END_TO_END_SUCCESS" if and only if
# the test(s) succeeded.

# Install the packages. Different methods on Fedora and CentOS.
if [ -f /etc/fedora-release ]; then
	sudo dnf copr -y enable oyvindh/Nivlheim-test
	sudo dnf install -y nivlheim-client nivlheim-server || touch installerror
elif [ -f /etc/centos-release ]; then
	sudo yum install -y epel-release
	sudo curl -o /etc/yum.repos.d/oyvindh-Nivlheim-test-epel-7.repo \
		https://copr.fedorainfracloud.org/coprs/oyvindh/Nivlheim-test/repo/epel-7/oyvindh-Nivlheim-test-epel-7.repo
	sudo yum install -y nivlheim-client nivlheim-server || touch installerror
fi
if [ -f installerror ]; then
	echo "Package installation failed."
	exit
fi

# Check that the home page is being served
if [ $(curl -sSk https://localhost/ | grep -c "<title>Nivlheim</title>") -eq 0 ]; then
	echo "The web server isn't properly configured and running."
	exit
fi

# Check that the API is available through the main web server
if ! curl -sSko /dev/null https://localhost/api/v0/status; then
	echo "The API is unavailable."
	exit
fi

# Configure the client to use the server at localhost
echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
# Run the client, it will be put on waiting list for a certificate
sudo /usr/sbin/nivlheim_client
# Approve the client, using the API
ID=`curl -sS 'http://localhost:4040/api/v0/awaitingApproval?fields=approvalId'|perl -ne 'print $1 if /"approvalId":\s+(\d+)/'`
curl -X PUT -sS "http://localhost:4040/api/v0/awaitingApproval/$ID?hostname=abcdef"

# Run the client again, this time it will receive a certificate
# and post data into the system
sudo /usr/sbin/nivlheim_client
if [ ! -f /var/nivlheim/my.crt ]; then
	echo "Certificate generation failed."
	exit
fi

# wait for server to process incoming data
OK=0
for try in {1..20}; do
	sleep 3
	# Query the API for the new machine
	if [ $(curl -sS 'http://localhost:4040/api/v0/hostlist?fields=hostname' | grep -c "abcdef") -gt 0 ]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "Home page does not show the new machine."
	exit
fi

echo "END_TO_END_SUCCESS"
