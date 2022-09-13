#!/bin/bash
echo "------------- Pull request requirements ---------------------"
set -e
cd `dirname $0`; cd ..
OK=1

echo -n "Required patch level in main.go matches the patch files: "
A=`grep -oP "const requirePatchLevel = \K[0-9]+" server/service/main.go`
B=`ls -1 server/service/database | wc -l`
if [[ "$A" == "$B" ]]; then
	echo "OK"
else
	echo "FAIL"
	OK=0
fi

echo -n "The version number has been updated in VERSION:          "
A=`cat VERSION`
if [ -f master/VERSION ]; then
	B=`cat master/VERSION`
else
	B=`git show master:VERSION`
fi
if [[ "$A" != "$B" ]]; then
	echo "OK"
else
	echo "FAIL"
	OK=0
fi

echo -n "Correct version number in the Linux client:              "
B=`grep -oP "VERSION = '\K[0-9.]+(?=')" client/nivlheim_client`
if [[ "$A" == "$B" ]]; then
	echo "OK"
else
	echo "FAIL"
	OK=0
fi

echo -n "Correct version number in the Powershell client:         "
B=`grep -oP 'Set-Variable version -option Constant -value "\K[0-9.]+(?=")' client/windows/nivlheim_client.ps1`
if [[ "$A" == "$B" ]]; then
	echo "OK"
else
	echo "FAIL"
	OK=0
fi

echo -n "Updated Debian changelog:                                "
if head -1 debian/changelog | grep -q -s $A; then
	echo "OK"
else
	echo "FAIL"
	OK=0
fi

if [[ "$OK" != "1" ]];
then
	exit 1
fi
