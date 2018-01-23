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

KEYPAIRNAME="jenkins_key"
while getopts k: option; do
	case "${option}"
	in
		k) KEYPAIRNAME=${OPTARG};;
	esac
done

# Create an array containing the available machine images
TMPFILE=$(mktemp)
openstack image list | grep active | cut -d '|' -f 3 > $TMPFILE
IFS=$'\r\n' GLOBIGNORE='*' command eval  'LIST=($(cat ${TMPFILE}))'
rm $TMPFILE

# Clean up the list, only keep the images we want to test on
IMAGES=()
for IMAGE in "${LIST[@]}"; do
	if [[ $IMAGE != *"Fedora"* ]] && [[ $IMAGE != *"CentOS 7"* ]]; then
		continue
	fi
	IMAGE=$(echo $IMAGE | xargs) # trim leading/trailing whitespace
	IMAGES+=("$IMAGE")
done

# If the list of machine images is empty, something is seriously wrong
if [ ${#IMAGES[@]} -eq 0 ]; then
	exit 1 # non-zero exit status will let Jenkins interpret this as a failure
fi

# set "pending" status on GitHub for all platforms
if [[ "$GITHUB_TOKEN" != "" ]] && [[ "$GIT_COMMIT" != "" ]]; then
	for IMAGE in "${IMAGES[@]}"; do
		curl -XPOST -H "Authorization: token $GITHUB_TOKEN" \
			https://api.github.com/repos/usit-gd/nivlheim/statuses/$GIT_COMMIT -d "{
			\"state\": \"pending\",
			\"target_url\": \"\",
			\"description\": \"Results are pending...\",
			\"context\": \"$IMAGE\"
		}" -sS -o /dev/null
	done
fi

for IMAGE in "${IMAGES[@]}"; do
	echo "Creating a VM with \"$IMAGE\""
	NAME="voyager"
	openstack server delete --wait $NAME 2>/dev/null # just to be sure
	openstack server create --image "$IMAGE" --flavor m1.small \
		--key-name $KEYPAIRNAME --nic net-id=dualStack --wait $NAME \
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

	OK=0
	if [[ $IP != "" ]]; then
		echo -n "Waiting for the VM to finish booting"
		for try in {1..20}; do
			if echo bleh | nc -w 2 $IP 22 1>/dev/null 2>&1; then
				OK=1
				break
			fi
			sleep 5
			echo -n ".."
		done
		echo ""
	fi
	if [[ ! $OK -eq 1 ]]; then
		echo "Unable to connect to the VM, giving up."
		LOGFILE=""
	else
		echo "Installing and testing packages"
		ssh $USER\@$IP -o StrictHostKeyChecking=no \
			-q -o UserKnownHostsFile=/dev/null \
			-C "cat > script" < $(dirname "$0")"/test_packages.sh"

		TIMESTAMP=$(date '+%Y%m%d-%H%M%S')
		LOGFILE=$(echo "${IMAGE}_${TIMESTAMP}.log" | sed -e 's/ /_/g')
		ssh $USER\@$IP -o StrictHostKeyChecking=no \
			-q -o UserKnownHostsFile=/dev/null \
			-C "chmod a+x script; ./script" > "$LOGFILE" 2>&1

		scp $LOGFILE oyvihag@callisto.uio.no:
	fi

	openstack server delete --wait $NAME

	if [[ "$GITHUB_TOKEN" != "" ]] && [[ "$GIT_COMMIT" != "" ]]; then
		STATUS="failure"
		URL=""
		if [[ "$LOGFILE" != "" ]] && [[ -f $LOGFILE ]]; then
			URL="https://folk.uio.no/oyvihag/logs/$LOGFILE"
			if [ $(grep -c END_TO_END_SUCCESS "$LOGFILE") -gt 0 ]; then
				STATUS="success"
			fi
		fi
		curl -XPOST -H "Authorization: token $GITHUB_TOKEN" \
			https://api.github.com/repos/usit-gd/nivlheim/statuses/$GIT_COMMIT -d "{
			\"state\": \"$STATUS\",
			\"target_url\": \"$URL\",
			\"description\": \"$STATUS\",
			\"context\": \"$IMAGE\"
		}" -sS -o /dev/null
	fi
done
