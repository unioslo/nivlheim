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
grep 'Version:' $SPEC

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

# Check the spec file for errors.
# rpmlint uses http HEAD requests to verify the source urls.
# Some web servers are misconfigured and return 403 (forbidden) for HEAD, even though GET works.
# Here we avoid false warnings by disabling networking.
rpmlint -i -f "$SOURCEDIR/rpmlint.conf" -o "NetworkEnabled False" $SPEC || exit 1

# Use spectool to download the source files
echo "buildrpm: Downloading source files"
cd $BUILDDIR/SOURCES
spectool -gf $SPEC > /dev/null 2>&1
if [ "$2" != "" ]; then
	# If we are about to replace the main source package with local files anyway,
	# then put a placeholder file for the source file.
	# This is useful in case you're working on a new branch that has not yet
	# been pushed to Github, because in that case the download doesn't work.
	touch $BUILDDIR/SOURCES/$GIT_BRANCH.tar.gz
fi
# Count the downloaded files to see if it worked. As an additional measure.
if [[ $(ls -1 $BUILDDIR/SOURCES | wc -l) -ne $(grep Source $SPEC | grep http | wc -l) ]]; then
	echo "buildrpm: Didn't manage to produce the source files."
	rpmbuild --nobuild --nodeps $SPEC
	exit 1
fi

# Replace the main source package with local files
if [ "$2" != "" ]; then
	cd $2
	echo "buildrpm: Replacing with source from $(pwd)"
	if [[ ! -f ./README.md ]]; then
		echo "This directory doesn't look like it contains the source!"
		exit 1
	fi
	tempdir=$(mktemp -d -t tmp.XXXXXXXXXX)
	mkdir $tempdir/nivlheim-$GIT_BRANCH
	cp -a * $tempdir/nivlheim-$GIT_BRANCH
	cd $tempdir
	tar -czf $BUILDDIR/SOURCES/$GIT_BRANCH.tar.gz nivlheim-$GIT_BRANCH
	cd $BUILDDIR
	rm -rf "$tempdir"
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
rpmlint -i -f "$SOURCEDIR/rpmlint.conf" $A || exit 1

srpm=`eval echo "~/rpmbuild/SRPMS/*.src.rpm"`
if [ ! -f $srpm ]
then
	echo "Can't find source rpm file ($sprm)"
	exit 1
fi

# mock build
config=$(basename -s .cfg $(readlink -f /etc/mock/default.cfg))
echo ""
echo "--------------------------------------------------------"
echo "  Mock-building packages for $config"
echo "--------------------------------------------------------"
if ! mock --bootstrap-chroot --rebuild $srpm; then
	grep -s BUILDSTDERR "/var/lib/mock/${config}-bootstrap/result/build.log"
	echo "Mock build failed for $config."
	echo "Re-trying with the old chroot method..."
	if ! mock --old-chroot --rebuild $srpm; then
		grep -s BUILDSTDERR "/var/lib/mock/$config/result/build.log"
		echo "Mock build with old chroot also failed for $config."
		exit 1
	fi
fi
rm -f /var/lib/mock/$config/result/*.src.rpm
echo ""
echo "--------------------------------------------------------"
echo "  rpmlint report"
echo "--------------------------------------------------------"
rpmlint -i -f "$SOURCEDIR/rpmlint.conf" /var/lib/mock/$config/result/*.rpm || exit 1
