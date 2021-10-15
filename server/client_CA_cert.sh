#!/bin/bash

# This script is a part of Nivlheim.
# It is used to create CA certificates that are used for signing
# client certificates.
#
# It is intended to be run without parameters, as a cron job.
# It will check the expiry date of the existing CA certificate,
# and replace it with a new one when necessary.
#
# To make it easier for 3rd party software to verify client certificates,
# a new CA certificate will appear in the bundle https://<server>/clientca.pem
# 3 weeks before it is actually put to use.


if [ `whoami` != "root" ]; then
	echo "This script must be run as root."
	exit 1
fi

# What operations to perform
CREATE=0
ACTIVATE=0
VERBOSE=0

# Parameters may override normal operations
while (( "$#" )); do
	if [[ "$1" == "--force-create" ]]; then
		CREATE=1
	elif [[ "$1" == "--force-activate" ]]; then
		ACTIVATE=1
	elif [[ "$1" == "--verbose" ]] || [[ "$1" == "-v" ]]; then
		VERBOSE=1
	else
		echo "Unknown argument: $1"
		exit 1
	fi
	shift
done

cd /var/www/nivlheim/CA

# If the CA certificate will expire in less than 30 days, create a new one
if [ ! -f nivlheimca.crt ] || ! openssl x509 -checkend 2592000 -noout -in nivlheimca.crt -enddate >/dev/null; then
	CREATE=1
fi

# If the CA certificate will expire in less than 9 days, change to the new one
if [ ! -f nivlheimca.crt ] || ! openssl x509 -checkend 777600 -noout -in nivlheimca.crt >/dev/null; then
	ACTIVATE=1
fi

if [[ $CREATE -eq 1 ]]; then
	if [ ! -f new_nivlheimca.crt ] || [ ! -f new_nivlheimca.key ]; then
		[ $VERBOSE -eq 1 ] && echo "Creating a new CA certificate"

		# Generate a new certificate
		rm -f old_*
		openssl genrsa -out new_nivlheimca.key 4096 >/dev/null 2>&1
		openssl req -new -key new_nivlheimca.key -out new_nivlheimca.csr -subj "/C=NO/ST=Oslo/L=Oslo/O=UiO/OU=USIT/CN=Nivlheim$RANDOM"
		openssl x509 -req -days 365 -in new_nivlheimca.csr -out new_nivlheimca.crt -signkey new_nivlheimca.key >/dev/null 2>&1

		# Fix permissions
		chgrp apache new_nivlheimca.*
		chmod 640 new_nivlheimca.key

		# Show results
		[ $VERBOSE -eq 1 ] && openssl x509 -in new_nivlheimca.crt -noout -enddate

		# create a bundle with the old and the new CA
		if [ -f nivlheimca.crt ]; then
			cat nivlheimca.crt new_nivlheimca.crt > clientca.pem
		else
			cp new_nivlheimca.crt clientca.pem
		fi
	else
		echo "Won't create a new CA certificate; One has already been created and is waiting"
	fi
fi

# Copy the certificate bundle to the web root
if [ -f clientca.pem ]; then
	cp -f clientca.pem /var/www/html/
fi

if [[ $ACTIVATE -eq 1 ]]; then
	if [ -f new_nivlheimca.crt ] && [ -f new_nivlheimca.key ]; then
		[ $VERBOSE -eq 1 ] && echo "Activating the new CA certificate"
		# Activate/change to the new CA certificate
		if [ -f nivlheimca.crt ]; then
			mv nivlheimca.key old_nivlheimca.key
			mv nivlheimca.csr old_nivlheimca.csr
			mv nivlheimca.crt old_nivlheimca.crt
		fi
		mv new_nivlheimca.key nivlheimca.key
		mv new_nivlheimca.csr nivlheimca.csr
		mv new_nivlheimca.crt nivlheimca.crt
		# signal httpd to gracefully restart, if it is running
		if [[ -f /var/run/httpd/httpd.pid ]]; then
			kill -usr1 `cat /var/run/httpd/httpd.pid`
		fi
	else
		echo "There's no new CA certificate to activate"
		exit 1
	fi
fi
