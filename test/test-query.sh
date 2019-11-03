#!/bin/bash
sudo apt-get install -y jq

echo "CIRCLE_BRANCH=$CIRCLE_BRANCH" > .env

docker-compose up -d --force-recreate
if [[ $? != 0 ]]; then
  docker-compose logs
  exit 1
fi

set -e

docker run --network container:thanos \
  appropriate/curl --retry 10 --retry-delay 5 --retry-connrefused http://localhost:10902/-/healthy

# We just check we get some JSON back for now...
docker run --network container:thanos \
  appropriate/curl --retry 2 --retry-delay 5 "http://localhost:10902/api/v1/query?query=test%3Aa%3A5&dedup=true&partial_response=true&time=1572629812.291&_=1572629811861" | jq .
