#!/bin/bash

# verify root
if [ `whoami` != "root" ]; then
	echo "This script must be run by the root user."
	exit 1
fi

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

# initialize postgresql
if ! /usr/bin/postgresql-setup --initdb; then
	echo "There is apparently an existing PostgreSQL installation."
fi

# restart apache httpd and postgres
if which systemctl > /dev/null 2>&1; then
	systemctl restart httpd
	systemctl restart postgresql
elif which service > /dev/null 2>&1; then
	service httpd restart
	service postgresql restart
fi

# create a database user that
# local httpd processes will automatically authenticate as,
# as long as Postgres is set up for peer authentication
sudo -u postgres bash -c "createuser apache"
sudo -u postgres bash -c "psql -c \"create database apache\""

# create tables
sudo -u apache bash -c "psql < /var/nivlheim/init.sql"
