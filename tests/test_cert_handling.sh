#!/bin/bash

# Dependencies:
# - A running Postgres server
# - The current user must have a default database/schema that is safe to overwrite
# - You should probably not run this as the httpd user, then

# tempdir
tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
function finish {
  rm -rf "$tempdir"
}
trap finish EXIT

# Clean the database
cd $(dirname $0)
cd ../server
psql -q -1 -v ON_ERROR_STOP=1 -f init.sql || exit 1

# Set up IP ranges
#curl -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=129.240.0.0/16&useDns=true'
#curl -X POST 'http://localhost:4040/api/v0/settings/ipranges' -d 'ipRange=193.157.111.0/24'
psql -q -c "INSERT INTO ipranges(iprange,use_dns) VALUES('129.240.0.0/16',true)" || exit 1
psql -q -c "INSERT INTO ipranges(iprange) VALUES('193.157.111.0/24')" || exit 1

# Request a certificate
REMOTE_ADDR=129.240.202.63 QUERY_STRING='hostname=abc.example.no' perl cgi/reqcert > $tempdir/cert || exit 1
if ! grep -qe "--BEGIN CERTIFICATE--" $tempdir/cert; then
	cat $tempdir/cert
	exit 1
fi
rm $tempdir/cert

# Verify that reqcert didn't leave any files
if [[ $(ls -1 /var/www/nivlheim/certs | wc -l) -gt 0 ]]; then
	ls -1 /var/www/nivlheim/certs
	exit 1
fi

# Grab the actual certificate text from the database table
psql --no-align -c "SELECT cert FROM certificates WHERE certid=1" > $tempdir/cert
export SSL_CLIENT_CERT=$(sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' < $tempdir/cert)

# Try secure/ping with the cert
export SSL_CLIENT_V_END='Jun 02 11:12:13 2049'
if [[ $(REMOTE_ADDR='129.240.202.63' perl cgi/ping2 | grep "pong" | wc -l) -lt 1 ]]; then
	echo "Secure/ping didn't work"
	exit 1
fi

# Try renewcert
REMOTE_ADDR='129.240.202.63' perl cgi/renewcert > $tempdir/cert2 || exit 1
if ! grep -qe "--BEGIN CERTIFICATE--" $tempdir/cert2; then
	cat $tempdir/cert2
	exit 1
fi
if [[ $(ls -1 /var/www/nivlheim/certs | wc -l) -gt 0 ]]; then
	ls -1 /var/www/nivlheim/certs
	exit 1
fi
export SSL_CLIENT_CERT=$(sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' < $tempdir/cert2)
# one more time
REMOTE_ADDR='129.240.202.63' perl cgi/renewcert > $tempdir/cert3 || exit 1
export SSL_CLIENT_CERT=$(sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p' < $tempdir/cert3)

# Check that the database table contains the certificate chain
chain=$(psql --no-align -t -c "SELECT certid,first,previous FROM certificates ORDER BY certid")
expect=$(echo -e "1|1|\n2|1|1\n3|1|2\n")
if [[ "$chain" != "$expect" ]]; then
	psql -c "SELECT certid,issued,first,previous,fingerprint FROM certificates ORDER BY certid"
	echo "$chain"
	exit 1
fi

# Blacklist and check response
psql -q -c "UPDATE certificates SET revoked=true"
if [[ $(REMOTE_ADDR='129.240.202.63' perl cgi/ping2 | grep "pong" | wc -l) -gt 0 ]]; then
	echo "Secure/ping worked even though cert was blacklisted."
	exit 1
fi
if [[ $(REMOTE_ADDR='129.240.202.63' perl cgi/renewcert | grep "Status: 403" | wc -l) -ne 1 ]]; then
	echo "renewcert worked even though cert was blacklisted."
	exit 1
fi
if [[ $(REMOTE_ADDR='129.240.202.63' perl cgi/post | grep "revoked" | wc -l) -ne 1 ]]; then
	echo "post worked even though cert was blacklisted."
	REMOTE_ADDR='129.240.202.63' perl cgi/post
	exit 1
fi
psql -q -c "UPDATE certificates SET revoked=false"

# Verify that post handles nonces correctly
psql -q -c "UPDATE certificates SET nonce=314, revoked=false"
REMOTE_ADDR='129.240.202.63' perl cgi/post nonce=517 > $tempdir/output
if [[ $(psql -t --no-align -c "select revoked from certificates where certid=3") != "t" ]]; then
	echo "Post failed to revoke cert when nonce wasn't correct."
	exit 1
fi
if ! grep -q -e "403" $tempdir/output; then
	echo "Post failed to reject when nonce wasn't correct."
	exit 1
fi

psql -q -c "UPDATE certificates SET nonce=314, revoked=false"
REMOTE_ADDR=129.240.202.63 perl cgi/post nonce=314 > $tempdir/output
if grep -q -e "403" $tempdir/output; then
	echo "Post failed to accept even though nonce was correct."
	exit 1
fi
if [[ $(psql -t --no-align -c "select revoked from certificates where certid=3") != "f" ]]; then
	echo "Post didn't accept correct nonce."
	exit 1
fi
