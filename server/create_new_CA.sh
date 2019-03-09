#!/bin/bash

# Check that I am running as root
if [ `whoami` != "root" ]; then
	echo "This script must be run as root."
	exit 1
fi

DAYS=365
if [ "$1" != "" ]; then
	DAYS=$1
fi

cd /var/www/nivlheim/CA
if [ -f new_nivlheimca.crt ] || [ -f new_nivlheimca.key ]; then
	echo "There is an existing new CA certificate waiting."
	exit 1
fi

# Generate a new certificate
rm -f old_*
openssl genrsa -out new_nivlheimca.key 4096
openssl req -new -key new_nivlheimca.key -out new_nivlheimca.csr -subj "/C=NO/ST=Oslo/L=Oslo/O=UiO/OU=USIT/CN=Nivlheim$RANDOM"
openssl x509 -req -days $DAYS -in new_nivlheimca.csr -out new_nivlheimca.crt -signkey new_nivlheimca.key

# Fix permissions
chgrp apache new_nivlheimca.*
chmod 640 new_nivlheimca.key

# Show results
openssl x509 -in new_nivlheimca.crt -noout -enddate

# create a bundle with the old and the new CA
cat nivlheimca.crt new_nivlheimca.crt > /var/www/html/clientca.pem
