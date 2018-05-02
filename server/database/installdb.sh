#!/bin/bash

# halt on errors
set -e

# go to the script directory, where the sql files are
cd $(dirname $0)

# Determine the current patch level
PATCHLEVEL=0
if P=$(psql -X -w -t --no-align -c "SELECT patchlevel FROM db" 2>/dev/null); then
	PATCHLEVEL=$P
fi
echo "Database patch level is $PATCHLEVEL"

# Applying patches as necessary
for P in $(seq -w 1 999); do
	if [[ $P -le $PATCHLEVEL ]]; then
		continue
	fi
	FILE="patch$P.sql"
	if [[ ! -f $FILE ]]; then
		break
	fi
	echo "Applying $FILE"
	psql -X -1 -w -v ON_ERROR_STOP=1 -f $FILE
done
