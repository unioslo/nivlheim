#!/bin/bash

# verify root
if [ `whoami` != "root" ]; then
	echo "This script must be run by the root user."
	exit 1
fi

# make dirs
mkdir -p /var/www/nivlheim/{db,certs,CA,queue}

# initialize certificate db
cd /var/www/nivlheim/db
touch index.txt
echo 'unique_subject = no' > index.txt.attr
echo '100001' > serial
touch /var/www/nivlheim/rand

# generate a Certificate Authority certificate to sign other certs with
cd /var/www/nivlheim/CA
if [ ! -f nivlheimca.key ]; then
	openssl genrsa -out nivlheimca.key 4096 -config /etc/nivlheim/openssl_ca.conf
	openssl req -new -key nivlheimca.key -out nivlheimca.csr -config /etc/nivlheim/openssl_ca.conf
	openssl x509 -req -days 365 -in nivlheimca.csr -out nivlheimca.crt -signkey nivlheimca.key
fi

# generate a SSL certificate as a default for the web server
cd /var/www/nivlheim
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

# fix permissions
chgrp -R apache /var/www/nivlheim /var/log/nivlheim
chmod -R g+w /var/log/nivlheim
chmod 0640 /var/www/nivlheim/default_key.pem
chmod 0644 /var/www/nivlheim/default_cert.pem
chcon -R -t httpd_sys_rw_content_t /var/log/nivlheim /var/www/nivlheim/{db,certs,rand,queue}
chown -R apache:apache /var/www/nivlheim/{db,certs,rand,queue}
chmod -R u+w /var/www/nivlheim/{db,certs,rand,queue}
setsebool httpd_can_network_connect_db on

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

# let the root user have access to the database too
sudo -u postgres bash -c "createuser root"
sudo -u postgres bash -c "psql -c \"grant apache to root\""

# create tables
sudo -u apache bash -c "psql < /var/nivlheim/init.sql"

# compile and install the Go code
rm -f /usr/sbin/nivlheim_jobs
rm -f /var/www/cgi-bin/frontpage.cgi
export GOPATH=/var/nivlheim/go
export GOBIN=$GOPATH/bin
#
cd $GOPATH/src/jobrunner
go get || exit 1
go install || exit 1
mv $GOBIN/jobrunner /usr/sbin/nivlheim_jobs
chcon -t bin_t -u system_u /usr/sbin/nivlheim_jobs
#
cd $GOPATH/src/web
go get || exit 1
go install || exit 1
mv $GOBIN/web /var/www/cgi-bin/frontpage.cgi
chcon -t httpd_sys_script_exec_t -u system_u /var/www/cgi-bin/frontpage.cgi

# enable the systemd service
if which systemctl > /dev/null 2>&1; then
	systemctl enable nivlheim
	systemctl restart nivlheim
fi
