#!/bin/bash

# How to use OpenStack and UH-IaaS

# 1. Sign up for UH-IaaS, and MAKE SURE TO WRITE DOWN THE API PASSWORD
# THAT IS SHOWN ONLY ONCE. Read about it here:
# http://docs.uh-iaas.no/en/latest/login.html#first-time-login

# 2. Install and configure the CLI tools:
# http://docs.uh-iaas.no/en/latest/api.html#openstack-command-line-interface-cli

# 3. You can now run commands to create/modify/delete VMs

source ~/keystone_rc.sh # provides environment variables with authentication information
source ~/github_rc.sh # provides GitHub API token

# Create an array containing the available machine images
TMPFILE=$(mktemp)
openstack image list | grep active | cut -d '|' -f 3 > $TMPFILE
IFS=$'\r\n' GLOBIGNORE='*' command eval  'IMAGES=($(cat ${TMPFILE}))'
rm $TMPFILE

for IMAGE in "${IMAGES[@]}"; do
	if [[ $IMAGE != *"Fedora"* ]] && [[ $IMAGE != *"CentOS 7"* ]]; then
		continue
	fi
	IMAGE=$(echo $IMAGE | xargs) # trim leading/trailing whitespace

	if [[ $GITHUB_TOKEN != "" ]] && [[ $GIT_COMMIT != "" ]]; then
		curl -XPOST -H "Authorization: token $GITHUB_TOKEN" \
			https://api.github.com/repos/usit-gd/nivlheim/statuses/$GIT_COMMIT -d "{
			\"state\": \"pending\",
			\"target_url\": \"\",
			\"description\": \"Results are pending...\",
			\"context\": \"$IMAGE\"
		}" -sS -o /dev/null
	fi

	echo "Creating a VM with \"$IMAGE\""
	NAME="voyager"
	openstack server create --image "$IMAGE" --flavor m1.small \
		--key-name jenkins_key --nic net-id=dualStack --wait $NAME \
		> /dev/null
	IP=$(openstack server list | grep $NAME | \
		grep -oE '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
	echo "IP address: \"$IP\""
	USER=root
	if [[ $IMAGE == *"CentOS"* ]]; then USER=centos; fi
	if [[ $IMAGE == *"Fedora"* ]]; then USER=fedora; fi
	if [[ $IMAGE == *"Ubuntu"* ]]; then USER=ubuntu; fi
	if [[ $IMAGE == *"Debian"* ]]; then USER=debian; fi
	if [[ $IMAGE == *"CirrOS"* ]]; then USER=cirros; fi
	echo "User: $USER"

	echo -n "Waiting for the VM to finish booting"
	until (echo bleh | nc -w 2 $IP 22 1>/dev/null 2>/dev/null); do
		sleep 5
		echo -n ".."
	done
	echo ""

	echo "Installing and testing packages"
	ssh $USER\@$IP -o StrictHostKeyChecking=no \
		-q -o UserKnownHostsFile=/dev/null \
		-C "cat > script" < $(dirname "$0")"test_packages.sh"

	LOGFILE="log $IMAGE.txt"
	ssh $USER\@$IP -o StrictHostKeyChecking=no \
		-q -o UserKnownHostsFile=/dev/null \
		-C "chmod a+x script; ./script" > $LOGFILE 2>&1

	openstack server delete --wait $NAME

	if [[ $GITHUB_TOKEN != "" ]] && [[ $GIT_COMMIT != "" ]]; then
		STATUS="failure"
		if [ grep -c END_TO_END_SUCCESS "$LOGFILE" -gt 0 ]; then
			STATUS="success"
		fi
		curl -XPOST -H "Authorization: token $GITHUB_TOKEN" \
			https://api.github.com/repos/usit-gd/nivlheim/statuses/$GIT_COMMIT -d "{
			\"state\": \"$STATUS\",
			\"target_url\": \"\",
			\"description\": \"$STATUS\",
			\"context\": \"$IMAGE\"
		}" -sS -o /dev/null
	fi
done
