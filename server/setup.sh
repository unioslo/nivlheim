#!/bin/bash

# verify root
if [ `whoami` != "root" ]; then
	echo "This script must be run by the root user."
	exit 1
fi

# download 3rd party Javascript and CSS libraries
cd /var/www/html/libs
./download_libraries.sh --prod && rm ./download_libraries.sh

# make dirs
mkdir -p /var/www/nivlheim/{db,certs,CA,queue}

# initialize certificate db
cd /var/www/nivlheim/db
touch index.txt
if [ ! -f index.txt.attr ]; then
	echo 'unique_subject = no' > index.txt.attr
fi
if [ ! -f serial ]; then
	echo '100001' > serial
fi
touch /var/www/nivlheim/rand

# generate a Certificate Authority certificate to sign other certs with
cd /var/www/nivlheim/CA
if [ ! -f nivlheimca.key ]; then
	openssl genrsa -out nivlheimca.key 4096
	openssl req -new -key nivlheimca.key -out nivlheimca.csr -config /etc/nivlheim/openssl_ca.conf
	openssl x509 -req -days 365 -in nivlheimca.csr -out nivlheimca.crt -signkey nivlheimca.key
fi

# generate a SSL certificate as a default for the web server
cd /var/www/nivlheim
if [ ! -f default_cert.pem ] || [ ! -f default_key.pem ]; then
	rm -f default_cert.pem default_key.pem csr
	# key
	openssl genpkey -outform PEM -out default_key.pem -algorithm RSA \
	  -pkeyopt rsa_keygen_bits:4096
	# certificate request
	openssl req -new -key default_key.pem -out csr -days 365 \
	  -subj "/C=NO/ST=Oslo/L=Oslo/O=UiO/OU=USIT/CN=localhost"
	# sign the request
	openssl ca -batch -in csr -cert CA/nivlheimca.crt -keyfile CA/nivlheimca.key \
	  -out default_cert.pem -config /etc/nivlheim/openssl_ca.conf
	rm -f csr
fi

# fix permissions
chgrp -R apache /var/www/nivlheim /var/log/nivlheim
chmod -R g+w /var/log/nivlheim
chmod 0640 /var/www/nivlheim/default_key.pem /var/www/nivlheim/CA/nivlheimca.key
chmod 0644 /var/www/nivlheim/default_cert.pem /var/www/nivlheim/CA/nivlheimca.crt
chcon -R -t httpd_sys_rw_content_t /var/log/nivlheim /var/www/nivlheim/{db,certs,rand,queue}
chown -R apache:apache /var/www/nivlheim/{db,certs,rand,queue}
chmod -R u+w /var/www/nivlheim/{db,certs,rand,queue}
setsebool -P httpd_can_network_connect_db on
setsebool -P httpd_can_network_connect on  # for proxy connections to the API

# initialize postgresql. new/old syntax
if ! (/usr/bin/postgresql-setup --initdb || /usr/bin/postgresql-setup initdb); then
	echo "Unable to initialize PostgreSQL database."
	echo "Assuming there is an existing installation."
fi

# restart apache httpd and postgres
systemctl restart httpd postgresql

# create a database user that
# local httpd processes will automatically authenticate as,
# as long as Postgres is set up for peer authentication
sudo -u postgres bash -c "createuser apache"
sudo -u postgres bash -c "psql -c \"create database apache\""

# let the root user have access to the database too
sudo -u postgres bash -c "createuser root"
sudo -u postgres bash -c "psql -c \"grant apache to root\""

# PostgreSQL Trigram extension 
sudo -u postgres psql -c "CREATE EXTENSION IF NOT EXISTS pg_trgm" apache

# update the database schema
sudo -u apache /var/nivlheim/installdb.sh

# start the Nivlheim service
systemctl restart nivlheim

# enable the services
systemctl enable httpd postgresql nivlheim
