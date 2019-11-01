#!/bin/bash
apt-get install -y jq

echo "CIRCLE_BRANCH=$CIRCLE_BRANCH" > .env

docker-compose up -d --force-recreate

i=10
while [[ $i -gt 0 ]]; do
  if curl http://localhost:10902/-/healthy | grep -q healthy; then
    break
  fi
  i=$[$i - 1]
  sleep 10
done

set -e

# We just check we get some JSON back for now...
curl "http://localhost:10902/api/v1/query?query=test%3Aa%3A5&dedup=true&partial_response=true&time=1572629812.291&_=1572629811861" | jq .
