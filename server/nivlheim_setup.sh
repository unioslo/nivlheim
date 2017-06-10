#!/bin/bash

# fix permissions
mkdir -p /var/www/nivlheim/{db,certs,CA}
chgrp -R apache /var/www/nivlheim
chmod -R g+w /var/www/nivlheim

# generate a Certificate Authority certificate for the client certificates
cd /var/www/nivlheim/CA
openssl genrsa -out nivlheimca.key 4096 -config /etc/nivlheim/openssl.cnf
openssl req -new -key nivlheimca.key -out nivlheimca.csr -config /etc/nivlheim/openssl_ca.cnf
openssl x509 -req -days 365 -in nivlheimca.csr -out nivlheimca.crt -signkey nivlheimca.key

# restart apache httpd
if which systemctl > /dev/null 2>&1; then
	systemctl restart httpd
elif which service > /dev/null 2>&1; then
	service httpd restart
fi
