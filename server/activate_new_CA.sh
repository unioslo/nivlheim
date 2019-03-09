#!/bin/bash

# This script is a part of Nivlheim.
# It activates a new CA certificate that has been previously created.

# Check that I am running as root
if [ `whoami` != "root" ]; then
	echo "This script must be run as root."
	exit 1
fi

cd /var/www/nivlheim/CA
if [ ! -f new_nivlheimca.crt ] || [ ! -f new_nivlheimca.key ]; then
	echo "There is no new CA certificate."
	exit 1
fi

echo "Activating the new CA certificate."
mv nivlheimca.key old_nivlheimca.key
mv nivlheimca.csr old_nivlheimca.csr
mv nivlheimca.crt old_nivlheimca.crt
mv new_nivlheimca.key nivlheimca.key
mv new_nivlheimca.csr nivlheimca.csr
mv new_nivlheimca.crt nivlheimca.crt
systemctl restart httpd
