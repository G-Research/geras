#!/bin/bash
docker-compose up -d --abort-on-container-exit --force-recreate

i=10
while [[ $i -gt 0 ]]; do
  if curl http://localhost:10902 | grep -q healthy; then
    break
  fi
  i=$[$i - 1]
  sleep 10
done

curl "http://localhost:10902/api/v1/query?query=test%3Aa%3A5&dedup=true&partial_response=true&time=1572629812.291&_=1572629811861"
