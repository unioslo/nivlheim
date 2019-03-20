#!/bin/bash

debuild -us -uc
dh_clean
dh_quilt_unpatch
rm -f debian/nivlheim-client.debhelper.log


