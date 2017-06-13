#!/bin/bash

# make dirs
mkdir -p /var/www/nivlheim/{db,certs,CA}

# initialize db
cd /var/www/nivlheim/db
touch index.txt
echo 'unique_subject = no' > index.txt.attr
echo '100001' > serial

# generate a Certificate Authority certificate for the client certificates
cd /var/www/nivlheim/CA
if [ ! -f nivlheimca.key ]; then
	openssl genrsa -out nivlheimca.key 4096 -config /etc/nivlheim/openssl_ca.conf
	openssl req -new -key nivlheimca.key -out nivlheimca.csr -config /etc/nivlheim/openssl_ca.conf
	openssl x509 -req -days 365 -in nivlheimca.csr -out nivlheimca.crt -signkey nivlheimca.key
fi

# generate a self-signed SSL certificate as a default for the web server
cd /var/www/nivlheim
rm -f default_cert.pem default_key.pem
openssl req -x509 -newkey rsa:4096 -keyout default_key.pem -out default_cert.pem\
 -days 365 -nodes -subj "/C=NO/ST=Oslo/L=Oslo/O=UiO/OU=USIT/CN=localhost"

# fix permissions
chgrp -R apache /var/www/nivlheim
chmod -R g+w /var/www/nivlheim

# restart apache httpd
if which systemctl > /dev/null 2>&1; then
	systemctl restart httpd
elif which service > /dev/null 2>&1; then
	service httpd restart
fi
