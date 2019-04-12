#!/bin/bash
# This script uses Openstack and UH-IaaS for aquiring VMs.
# See also: run_tests_on_vms.sh

# We want the script to exit with a non-zero value if any command fails
set -e

# Read parameters and set environment variables
source ~/keystone_rc.sh # provides environment variables with authentication information
OS_REGION_NAME=bgo # Use the Bergen region instead, for they have the best Windowses
KEYPAIRNAME="jenkins_key"
SECGROUP=""
while getopts "s:k:" option; do
	case "${option}"
	in
		k) KEYPAIRNAME=${OPTARG};;
		s) SECGROUP="${OPTARG}";;
	esac
done

# Define some utility functions
function scpnokey() {
	scp -q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PasswordAuthentication=no "$@"
}

function sshnokey() {
	ssh -q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PasswordAuthentication=no "$@"
}

# Provision a Windows VM to run the Powershell client on
echo "Creating a Windows VM..."
WIN="Windozer"
openstack server delete --wait $WIN 2>/dev/null || true # just to be sure
openstack server create --flavor win.medium --image "GOLD Windows Server 2019 Core" --nic net-id=dualStack --key-name $KEYPAIRNAME --wait $WIN
WINIP=$(openstack server list | grep $WIN | grep -oE '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
echo "IP address: \"$WINIP\""

echo "Creating a Fedora VM..."
FED="Trilby"
openstack server delete --wait $FED 2>/dev/null || true # just to be sure
openstack server create --flavor m1.small --image "GOLD Fedora 28" --nic net-id=dualStack --key-name $KEYPAIRNAME --wait $FED
FEDIP=$(openstack server list | grep $FED | grep -oE '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
echo "IP address: \"$FEDIP\""

# Ensure VMs always get deleted
function finish {
	openstack server delete --wait $WIN 2>/dev/null || true
	openstack server delete --wait $FED 2>/dev/null || true
}
trap finish EXIT

# Add security groups
echo "Adding security groups..."
openstack server add security group $WIN "OSL region" || true
openstack server add security group $FED "OSL region" || true
openstack server add security group $WIN "BGO region" || true
openstack server add security group $FED "BGO region" || true
if [[ "$SECGROUP" != "" ]]; then
	openstack server add security group $WIN "$SECGROUP" || true
	openstack server add security group $FED "$SECGROUP" || true
fi

# Wait for both VMs to finish booting
bootstart=`date +%s`
OK=0
if [[ "$WINIP" != "" ]] && [[ "$FEDIP" != "" ]]; then
	echo -n "Waiting for both VMs to finish booting"
	W=0
	F=0
	for try in {1..119}; do
		if sshnokey -o ConnectTimeout=2 admin\@$WINIP -C "echo OK" 1>/dev/null 2>&1; then
			W=1
		fi
		if echo bleh | nc -w 2 $FEDIP 22 1>/dev/null 2>&1; then
			F=1
		fi
		if [[ $W -eq 1 ]] && [[ $F -eq 1 ]]; then
			OK=1
			break
		fi
		sleep 5
		echo -n "."
	done
	echo ""
fi
if [[ ! $OK -eq 1 ]]; then
	echo "Unable to connect to both VMs, giving up."
	openstack server show $WIN
	openstack server show $FED
	exit 2
fi
bootend=`date +%s`
boottime=$((bootend-bootstart))
echo "That only took $boottime seconds, good job."

# Install the Nivlheim server package
sshnokey fedora\@$FEDIP -C "sudo dnf copr -y enable oyvindh/Nivlheim-test"
sshnokey fedora\@$FEDIP -C "sudo dnf install -y nivlheim-server"

# Pre-approve the Windows machine
curl -sSk --data "hostname=foo.example.com&ipAddress=$WINIP&approved=true" "https://$FEDIP/api/v2/manualApproval"

# transfer the Powershell script
cd "$(dirname "$0")"
scpnokey nivlheim_client.ps1 admin\@$WINIP:

# import the config to registry
encoded=$(base64 --wrap=0 client.conf)
sshnokey admin\@$WINIP -C "reg add HKEY_LOCAL_MACHINE\\SOFTWARE\\Nivlheim /f /v config /t REG_SZ /d $encoded"

# run the script with -certfile and -logfile pointing to a place the script is allowed to write
sshnokey admin\@$WINIP -C "powershell -Command ./nivlheim_client.ps1 -certfile nivlheim.p12 -logfile nivlheim.log -serverbaseurl https://$FEDIP/cgi-bin/ -trustallcerts:1"

# verify that the Powershell script actually sent some data
OK=0
for try in {1..20}; do
	sleep 3
	# Query the API for the new machine
	if [ $(curl -sSk "https://$FEDIP/api/v2/hostlist?fields=hostname" | grep -ci "foo.example.com") -gt 0 ]; then
		OK=1
		break
	fi
done
if [ $OK -eq 0 ]; then
	echo "The new Windows machine did not show up in Nivlheim."
	exit 1
fi
echo "Found the new VM in Nivlheim."

echo "Test result: OK"