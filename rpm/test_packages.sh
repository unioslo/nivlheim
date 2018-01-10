#!/bin/bash
set -x

if [ -f /etc/fedora-release ]; then
	sudo dnf copr enable oyvindh/Nivlheim-test
	sudo dnf install -y nivlheim-client nivlheim-server || touch installerror
elif [ -f /etc/redhat-release ]; then
	sudo yum install -y epel-release
	sudo curl -o /etc/yum.repos.d/oyvindh-Nivlheim-test-epel-7.repo \
		https://copr.fedorainfracloud.org/coprs/oyvindh/Nivlheim-test/repo/epel-7/oyvindh-Nivlheim-test-epel-7.repo
	sudo yum install -y nivlheim-client nivlheim-server || touch installerror
fi

if [ -f installerror ]; then
	echo "Package installation failed."
	exit
fi
