#!/bin/bash

# exit immediately on error
set -e

# go to the root dir of the repo
cd $(dirname $0)/../..
ROOT=$(pwd)

# clean up on exit, even if something fails
function cleanup {
	set +e
	cd $ROOT/ci/docker
	docker compose down
	docker stop postal 2>/dev/null
	docker rmi nivlheim nivlheim-www nivlheimclient
	rm -f ../../tmp_client.yaml
	echo "cleanup done."
}
trap cleanup EXIT

# go test
docker run --rm -p 5432:5432 --name postal -e POSTGRES_USER=nivlheim -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=nivlheim -d postgres:latest
echo "Waiting for Postgres to become ready..."
while ! docker exec postal pg_isready; do sleep 2; done
cd $ROOT/server/service
NIVLHEIM_PGHOST=localhost NIVLHEIM_PGUSER=nivlheim NIVLHEIM_PGPASSWORD=postgres NIVLHEIM_PGDATABASE=nivlheim NIVLHEIM_PGSSLMODE=disable go test -v
docker stop postal

# build images
cd $ROOT
docker build --file ci/docker/api_Dockerfile --tag nivlheim:latest .
docker build --file ci/docker/Dockerfile --tag nivlheim-www:latest .
cp client/client.yaml tmp_client.yaml
echo "  server: localhost" >> tmp_client.yaml
docker build --file ci/docker/client_Dockerfile --tag nivlheimclient:latest .

# run through all the test scripts
for a in tests/*.sh; do
 	docker compose -f "ci/docker/docker-compose.yml" up -d
 	sleep 5
 	echo $a
 	$a
 	docker compose -f "ci/docker/docker-compose.yml" down
	sleep 1
	docker volume prune -f
	docker volume rm docker_data docker_logs clientvar 2>/dev/null && true
done

echo "====----~~~~====----~~~~====----~~~~====----~~~~"
echo "EVERYTHING WORKS!!!!!!!!!!!!!!!!!!!!!!!"
echo "====----~~~~====----~~~~====----~~~~====----~~~~"
