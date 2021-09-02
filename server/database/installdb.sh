#!/bin/bash

# halt on errors
set -e

# read server.conf and set env vars for database connection parameters
if [[ -r "/etc/nivlheim/server.conf" ]]; then
	# grep out the postgres config options and make the names upper case
	grep -ie "^pg" /etc/nivlheim/server.conf | sed -e 's/\(.*\)=/\U\1=/' > /tmp/dbconf
	source /tmp/dbconf
	rm /tmp/dbconf
else
	echo "Unable to read server.conf"
	exit 1
fi
export PGHOST PGPORT PGDATABASE PGUSER PGPASSWORD

# go to the script directory, where the sql files are
cd $(dirname $0)

# Verify that I am able to connect to the database
if ! psql -X -w -c "SELECT version()" >/dev/null; then
	echo "Unable to connect to the database"
	exit 1
fi

# Determine the current patch level
PATCHLEVEL=0
if P=$(psql -X -w -t --no-align -c "SELECT patchlevel FROM db" 2>/dev/null); then
	PATCHLEVEL=$P
fi
echo "Database patch level is $PATCHLEVEL"

if [[ "$1" == "--wipe" ]]; then
	PATCHLEVEL=0
	echo "Wiping existing database"
fi

# Applying patches as necessary
for P in $(seq -w 1 999); do
	if [[ $P -le $PATCHLEVEL ]]; then
		continue
	fi
	FILE="../service/database/patch$P.sql"
	if [[ ! -f $FILE ]]; then
		break
	fi
	echo "Applying $FILE"
	psql -X -1 -w -q -v ON_ERROR_STOP=1 --pset pager=off -f $FILE
	# See also: http://petereisentraut.blogspot.com/2010/03/running-sql-scripts-with-psql.html
done
