#!/bin/bash
#====================================================
#  Required packages: rpmdevtools, rpmlint, mock, git
#====================================================

SOURCEDIR=$1
BUILDDIR=~/rpmbuild

# cleanup and prepare directories
rm -rf $BUILDDIR
mkdir -p $BUILDDIR
cd $BUILDDIR
mkdir BUILD BUILDROOT RPMS SOURCES SPECS SRPMS

# prepare spec and patch files
cd "$SOURCEDIR"
if [ ! -f *.spec ]
then
	echo "buildrpm: Missing spec file!"
	exit 1
fi
cp *.spec $BUILDDIR/SPECS/
cp *.patch $BUILDDIR/SOURCES/ 2>/dev/null
SPEC=`eval echo $BUILDDIR/SPECS/*.spec`
echo "buildrpm: Spec file = $SPEC"

# hard-replace a few macroes in the spec file, because
# otherwise the environment variables would have to be present
# during mock build later, which they aren't
cd "$SOURCEDIR"
export GIT_TAG=`git describe --tags --abbrev=0`
echo "buildrpm: The git tag is $GIT_TAG"
sed -i -e "s|%{getenv:GIT_TAG}|$GIT_TAG|g" $SPEC
export GIT_BRANCH=`echo $GIT_BRANCH | sed 's#.*/##'`
echo "buildrpm: The git branch is $GIT_BRANCH"
sed -i -e "s|%{getenv:GIT_BRANCH}|$GIT_BRANCH|g" $SPEC

# Check the spec file for errors
rpmlint -i $SPEC || exit 1

if [ "$2" = "" ]; then
	echo "buildrpm: Downloading source archive file"
	cd $BUILDDIR/SOURCES
	spectool -g $SPEC > /dev/null
	if [ ! -f $BUILDDIR/SOURCES/* ]; then
		echo "buildrpm: Didn't manage to produce the source archive file."
		rpmbuild --nobuild --nodeps $SPEC
		exit 1
	fi
else
	echo "buildrpm: Using source from $2"
	mkdir /tmp/nivlheim-$GIT_BRANCH
	cp -a $2/* /tmp/nivlheim-$GIT_BRANCH
	cd /tmp
	tar -czf $BUILDDIR/SOURCES/$GIT_BRANCH.tar.gz nivlheim-$GIT_BRANCH
	rm -rf /tmp/nivlheim-$GIT_BRANCH
fi

echo "buildrpm: Building source rpm"
cd $BUILDDIR
rpmbuild -bs \
	--define "_source_filedigest_algorithm md5" \
	--define "_binary_filedigest_algorithm md5" \
	$SPEC
if [ $? -ne 0 ]; then
	exit 1
fi

A=$BUILDDIR/SRPMS/*
rpmlint -i $A || exit 1

srpm=`eval echo "~/rpmbuild/SRPMS/*.src.rpm"`
if [ ! -f $srpm ]
then
	echo "Can't find source rpm file ($sprm)"
	exit 1
fi

# mock build
configs=(fedora-27-x86_64)
for config in "${configs[@]}"
do
	echo ""
	echo "--------------------------------------------------------"
	echo "  Mock-building packages for $config"
	echo "--------------------------------------------------------"
	mock --root=$config --quiet --clean
	mock --bootstrap-chroot --root=$config --quiet --clean
	if ! mock --bootstrap-chroot --root=$config --quiet --rebuild $srpm; then
		echo "Mock build failed for $config."
		echo "------------ build.log -------------"
		cat /var/lib/mock/$config/result/build.log
		echo "------------------------------------"
		exit 1
	fi
	rm -f /var/lib/mock/$config/result/*.src.rpm
	rpmlint -i /var/lib/mock/$config/result/*.rpm || exit 1
	echo ""
done
