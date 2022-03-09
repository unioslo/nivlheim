#!/bin/bash

# verify root
if [ `whoami` != "root" ]; then
	echo "This script must be run by the root user."
	exit 1
fi

# make dirs
mkdir -p /var/www/nivlheim/{db,certs,CA,queue}
mkdir -p /var/log/nivlheim
mkdir -p /var/log/httpd

# initialize certificate db
cd /var/www/nivlheim/db
if [ ! -f index.txt ]; then
	touch index.txt
	if [ ! -f index.txt.attr ]; then
		echo 'unique_subject = no' > index.txt.attr
	fi
	if [ ! -f serial ]; then
		echo '100001' > serial
	fi
	touch /var/www/nivlheim/rand
fi

# fix permissions
chgrp -R apache /var/www/nivlheim /var/log/nivlheim
chmod -R g+w /var/log/nivlheim
chown -R apache:apache /var/www/nivlheim/{db,certs,rand,queue}
chmod -R u+w /var/www/nivlheim/{db,certs,rand,queue}
chown -R root:apache /var/log/httpd
chmod -R 0770 /var/log/httpd

# update CA certificate if necessary
/usr/bin/client_CA_cert.sh --verbose

# generate a SSL certificate as a default for the web server
cd /var/www/nivlheim
if [ ! -f default_cert.pem ] || [ ! -f default_key.pem ]; then
	rm -f default_cert.pem default_key.pem csr
	# key
	openssl genpkey -outform PEM -out default_key.pem -algorithm RSA \
	  -pkeyopt rsa_keygen_bits:4096
	# certificate request
	export COMMONNAME=localhost
	openssl req -new -key default_key.pem -out csr -days 365 \
	  -subj "/C=NO/ST=Oslo/L=Oslo/O=UiO/OU=USIT/CN=localhost"
	# sign the request
	openssl ca -batch -in csr -cert CA/nivlheimca.crt -keyfile CA/nivlheimca.key \
	  -out default_cert.pem -config /etc/nivlheim/openssl_ca.conf
	rm -f csr
fi

# pass signals on to the httpd process
function sigterm()
{
	echo "Received SIGTERM"
	kill -term `cat /var/run/httpd/httpd.pid`
}
trap sigterm SIGTERM

# start web server
echo "Starting httpd"
httpd -D FOREGROUND &
wait $! # must do it this way to be able to forward signals
