#!/bin/bash

# This script downloads 3rd party Javascript and CSS libraries
# that the website uses.
# These libraries do not come with the Git repository,
# but are downloaded during the build process and added to the packages.

# As a developer, you can run this script to download the libraries
# into the libs/ folder.

set -e
cd `dirname $0`
echo "Downloading Javascript and CSS libraries into `pwd`"
OPTS="-sSfOL --retry 10"

# clean
rm -rf *.js *.css *.map fontawesome*

echo "Handlebars"
curl $OPTS https://cdnjs.cloudflare.com/ajax/libs/handlebars.js/4.7.7/handlebars.min.js
echo "jQuery"
curl $OPTS https://code.jquery.com/jquery-3.6.0.min.js
echo "Moment"
curl $OPTS https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.22.2/moment.min.js
echo "Bulma"
curl $OPTS https://cdn.jsdelivr.net/npm/bulma@0.9.3/css/bulma.min.css
echo "Tarantino"
curl $OPTS https://raw.githubusercontent.com/CodeYellowBV/tarantino/master/build/tarantino.min.js

echo "Font Awesome"
curl $OPTS https://use.fontawesome.com/releases/v5.2.0/fontawesome-free-5.2.0-web.zip
unzip -q fontawesome-free-5.2.0-web.zip
#  https://fontawesome.com/how-to-use/on-the-web/setup/hosting-font-awesome-yourself
mkdir fontawesome fontawesome/css
mv fontawesome-free-5.2.0-web/webfonts fontawesome/
mv fontawesome-free-5.2.0-web/css/all.css fontawesome/css/
rm -rf fontawesome-free-5.2.0-web fontawesome-free-5.2.0-web.zip
