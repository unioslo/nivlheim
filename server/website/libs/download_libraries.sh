#!/bin/bash

# This script downloads 3rd party Javascript and CSS libraries
# that the website uses.
# These libraries do not come with the Git repository,
# but are downloaded during the build process and added to the packages.

# As a developer, you can run this script to download the libraries
# into the libs/ folder.


# <script src="https://code.jquery.com/jquery-3.3.1.min.js"
#	integrity="sha256-FgpCb/KJQlLNfOu91ta32o/NMZxltwRo8QtmkMRdAu8="
#	crossorigin="anonymous"></script>
# <script type="text/javascript" src="https://cdnjs.cloudflare.com/ajax/libs/handlebars.js/4.0.11/handlebars.min.js"></script>
# <script type="text/javascript" src="https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.20.1/moment.min.js"></script>
# <script defer src="https://use.fontawesome.com/releases/v5.0.6/js/all.js"></script>
# <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/bulma/0.6.2/css/bulma.min.css">

cd `dirname $0`
echo "Downloading Javascript and CSS libraries into `pwd`"

curl -s --remote-name https://code.jquery.com/jquery-3.3.1.min.js
curl -s --remote-name https://cdnjs.cloudflare.com/ajax/libs/handlebars.js/4.0.11/handlebars.min.js
curl -s --remote-name https://cdnjs.cloudflare.com/ajax/libs/handlebars.js/4.0.11/handlebars.runtime.min.js
curl -s --remote-name https://cdnjs.cloudflare.com/ajax/libs/moment.js/2.20.1/moment.min.js
curl -s --remote-name https://cdnjs.cloudflare.com/ajax/libs/bulma/0.6.2/css/bulma.min.css
curl -s --remote-name https://cdnjs.cloudflare.com/ajax/libs/bulma/0.6.2/css/bulma.min.css.map

curl -s --remote-name https://use.fontawesome.com/releases/v5.0.6/fontawesome-free-5.0.6.zip
rm -rf fontawesome-free-5.0.6 fontawesome
unzip -q fontawesome-free-5.0.6.zip
mv fontawesome-free-5.0.6/on-server fontawesome
rm -rf fontawesome-free-5.0.6 fontawesome-free-5.0.6.zip
