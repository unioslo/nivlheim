#!/bin/bash
set -e
cd `dirname $0`
cd ../client/windows
PSQL=../../ci/docker/psql.sh

# Ensure cleanup
function cleanup {
	echo "Cleanup..."
	set +e
	docker stop pwsh >/dev/null 2>&1
	docker rm -f pwsh >/dev/null 2>&1
	rm -f /tmp/nivlheimca.crt
	echo "All done."
}
trap cleanup EXIT

# The following line is not necessary when running on a GitHub runner, but when running locally you might keep a server between runs
$PSQL -c "delete from files; delete from hostinfo; delete from certificates"

# Fetch the CA certificate from the server.
# It was used to sign the web server ssl certificate.
docker cp docker_nivlheimweb_1:/var/www/nivlheim/CA/nivlheimca.crt /tmp

# Create a Powershell container (but don't start it)
docker create --rm -t --network host --name pwsh mcr.microsoft.com/powershell pwsh -Command 'sleep 120'
docker cp /tmp/nivlheimca.crt pwsh:/usr/local/share/ca-certificates/
docker cp nivlheim_client.ps1 pwsh:/
docker cp test.conf pwsh:/client.conf

# Whitelist the private network address ranges
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=192.168.0.0/16'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=172.16.0.0/12'
curl -sS -X POST 'http://localhost:4040/api/v2/settings/ipranges' -d 'ipRange=10.0.0.0/8'

# Start the container
docker start pwsh >/dev/null

# Update the CA certificates in the container so the Nivlheim CA is in effect
docker exec pwsh pwsh -Command 'update-ca-certificates'
docker exec pwsh pwsh -Command 'Invoke-Webrequest -Uri "https://localhost/cgi-bin/ping"'

# Run the client
docker exec pwsh pwsh -Command '/nivlheim_client.ps1 -testmode:1'

# Trigger the job that gives the machine a hostname
echo "Triggering a job so Nivlheim will assign a hostname"
curl -sSf -X POST 'http://localhost:4040/api/internal/triggerJob/handleDNSchangesJob'

# Verify that the server got the data
echo "Waiting for the machine to show up in hostinfo"
OK=0
for try in {1..20}; do
	sleep 3
	echo -n "."
	# Query the API for the new machine
	if [ $(curl -sS 'http://localhost:4040/api/v2/hostlist?fields=hostname' | grep -c "hostname") -gt 0 ]; then
		OK=1
		echo "got it."
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The machine never showed up in hostinfo."
	exit 1
fi
echo ""

# Set the hostname to the commonname, to prevent a renewal of the certificate, just in case
$PSQL -c "UPDATE hostinfo SET hostname=commonname, os_hostname=commonname FROM certificates WHERE certfp=fingerprint"

# Second run
echo "Run the client again..."
docker exec pwsh pwsh -Command '/nivlheim_client.ps1 -testmode:1'
A=$($PSQL --no-align -t -c "select count(*) from certificates" | tr -d '\r\n')
if (($A != 1)); then
	echo "Expected 1 new certificate after 2 runs; ended up with $A."
	# Print client certificate details
	$PSQL -c "select commonname, fingerprint, nonce, revoked from certificates"
	$PSQL -c "select hostname, os_hostname, certfp from hostinfo"
	exit 1
fi

# Third run
echo "Provoke a renewal of the cert. Do this by changing the hostname in the database."
$PSQL -c "UPDATE hostinfo SET hostname='abcdef'"
docker exec pwsh pwsh -Command '/nivlheim_client.ps1 -testmode:1'

# Verify that the version hardcoded in the Powershell script is equal to the version found in the VERSION file in the repository
V1=$(cat ../../VERSION)
V2=$(grep "Set-Variable version" nivlheim_client.ps1 | awk '{print $6}' | tr -d '"')
if [[ "$V1" != "$V2" ]]; then
	echo ""
	echo "Version mismatch!"
	echo "The version hardcoded in the Powershell script is $V2, but the version found in the VERSION file in the repository is $V1"
	exit 1
fi

echo "Everything worked!"
