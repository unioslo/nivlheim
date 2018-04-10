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
SECGROUP=""
while getopts "s:k:" option; do
	case "${option}"
	in
		k) KEYPAIRNAME=${OPTARG};;
		s) SECGROUP="--security-group ${OPTARG}";;
	esac
done

cd $(dirname "$0")

# Create an array containing the available machine images
TMPFILE=$(mktemp)
openstack image list | grep active | cut -d '|' -f 3 | sort -u > $TMPFILE
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

EXITCODE=0

for IMAGE in "${IMAGES[@]}"; do
	echo "Creating a VM with \"$IMAGE\""
	NAME="voyager"
	openstack server delete --wait $NAME 2>/dev/null # just to be sure

	# UH-IaaS suddenly decided to keep more than 1 active image with the same name.
	# That means we can't use the image name for the "create server"-command.
	# This is a workaround.
	ID=$(openstack image list | grep "$IMAGE" | grep active | head -1 | cut -d '|' -f 2 | xargs)
	if [[ "$ID" == "" ]]; then
		echo "Could find image ID for $IMAGE :-("
		exit 1
	fi
	openstack server create --image "$ID" --flavor m1.small \
		--key-name $KEYPAIRNAME --nic net-id=dualStack \
		$SECGROUP --wait $NAME > /dev/null

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

	bootstart=`date +%s`
	OK=0
	if [[ $IP != "" ]]; then
		echo -n "Waiting for the VM to finish booting"
		for try in {1..119}; do
			if echo bleh | nc -w 2 $IP 22 1>/dev/null 2>&1; then
				OK=1
				break
			fi
			# UH-IaaS is unreliable, sometimes you have to stop/start the VM
			# before it will let you connect
			if [[ $(expr $try % 20) -eq 0 ]]; then
				echo -n "o"
				openstack server stop $NAME
				sleep 10
				echo -n "O"
				openstack server start $NAME
			fi
			sleep 5
			echo -n "."
		done
		echo ""
	fi
	if [[ ! $OK -eq 1 ]]; then
		echo "Unable to connect to the VM, giving up."
		openstack server show $NAME
		LOGFILE=""
		EXITCODE=2
	else
		bootend=`date +%s`
		boottime=$((bootend-bootstart))
		echo "That only took $boottime seconds, good job."
		echo "Installing and testing packages"
		scp -o StrictHostKeyChecking=no \
			-o UserKnownHostsFile=/dev/null \
			-q -p test_packages.sh $USER\@$IP:

		TIMESTAMP=$(date '+%Y%m%d-%H%M%S')
		LOGFILE=$(echo "${IMAGE}_${TIMESTAMP}.log" | sed -e 's/ /_/g')
		ssh $USER\@$IP -o StrictHostKeyChecking=no \
			-q -o UserKnownHostsFile=/dev/null \
			-C "./test_packages.sh || echo 'FAIL'" > "$LOGFILE" 2>&1

		scp -q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
			../tests/* $USER\@$IP:
		for T in ../tests/*; do
			ssh $USER\@$IP -o StrictHostKeyChecking=no \
				-q -o UserKnownHostsFile=/dev/null \
				-C "~/$(basename $T) || echo 'FAIL'" >> "$LOGFILE" 2>&1
		done

		if grep -s "FAIL" "$LOGFILE"; then
			echo $(grep -c "FAIL" "$LOGFILE") "FAIL(s)"
		fi

		if [[ $(whoami) == "jenkins" ]]; then
			scp $LOGFILE oyvihag@callisto.uio.no:
		fi
	fi

	openstack server delete --wait $NAME

	if [[ "$GITHUB_TOKEN" != "" ]] && [[ "$GIT_COMMIT" != "" ]]; then
		STATUS="failure"
		URL=""
		if [[ "$LOGFILE" != "" ]] && [[ -f $LOGFILE ]]; then
			URL="https://folk.uio.no/oyvihag/logs/$LOGFILE"
			if ! grep -s "FAIL" "$LOGFILE"; then
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
		if [[ "$STATUS" == "failure" ]]; then
			EXITCODE=1
		fi
	fi
done

exit $EXITCODE
