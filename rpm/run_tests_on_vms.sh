#!/bin/bash

# How to use OpenStack and UH-IaaS

# 1. Sign up for UH-IaaS, and MAKE SURE TO WRITE DOWN THE API PASSWORD
# THAT IS SHOWN ONLY ONCE. Read about it here:
# http://docs.uh-iaas.no/en/latest/login.html#first-time-login

# 2. Install and configure the CLI tools: 
# http://docs.uh-iaas.no/en/latest/api.html#openstack-command-line-interface-cli

# 3. You can now run commands to create/modify/delete VMs


source ~/keystone_rc.sh # provides environment variables with authentication information

openstack image list | grep active | cut -d '|' -f 3 | while read IMAGE
do
	if [[ $IMAGE != *"Fedora"* ]] && [[ $IMAGE != *"CentOS 7"* ]]; then
		continue
	fi

	echo "Creating a VM with \"$IMAGE\""
	NAME="voyager"
	openstack server create --image "$IMAGE" --flavor m1.small \
		--key-name oyvihag_usit_key --nic net-id=dualStack --wait $NAME
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

	echo "Waiting for the VM to finish booting"
	until (echo bleh | nc -w 2 $IP 22); do
		sleep 10
	done

	LOCALFILE="test_packages.sh"
	ssh $USER\@$IP -o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		-C "cat > $LOCALFILE" < $LOCALFILE

	ssh $USER\@$IP -o StrictHostKeyChecking=no \
		-o UserKnownHostsFile=/dev/null \
		-C "chmod a+x $LOCALFILE; ./$LOCALFILE" > "log $IMAGE.txt" 2>&1

	openstack server delete --wait $NAME
done
