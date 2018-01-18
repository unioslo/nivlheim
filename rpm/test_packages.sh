#!/bin/bash
set -x

# This is intended to be an end-to-end test of the server and client packages.
# It should signal success by outputting "END_TO_END_SUCCESS" if and only if
# the test(s) succeeded.

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

if [ $(curl -s -k https://localhost/ | grep -c "Nivlheim") -eq 0 ]; then
	echo "The web server isn't properly configured and running."
	exit
fi

echo "server=localhost" | sudo tee -a /etc/nivlheim/client.conf
sudo /usr/sbin/nivlheim_client
sudo -u apache psql -c 'update waiting_for_approval set approved=true;'
sudo /usr/sbin/nivlheim_client
if [ ! -f /var/nivlheim/my.crt ]; then
	echo "Certificate generation failed."
	exit
fi

sleep 10 # wait for server to process incoming data
if [ $(curl -s -k https://localhost/ | grep -c "novalocal") -eq 0 ]; then
	echo "Home page does not show the new machine."
	exit
fi

echo "END_TO_END_SUCCESS"
