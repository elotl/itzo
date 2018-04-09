#!/bin/bash

set -e

if [ $# -lt 1 ]; then
    echo "Usage: $0 operation"
    exit 1
fi

unitname="helloserver"

if [[ $1 == "deploy" ]]; then
    echo "deploy"
    path=http://localhost:8000/rest/v1/deploy/$unitname
    data="image=bcox9000/helloserver"
    curl -X POST -d "$data" $path
elif [[ $1 == "start" ]]; then
    path=http://localhost:8000/rest/v1/start/$unitname
    data="command=/helloserver"
    echo "start"
    curl -X PUT -d "$data" $path
elif [[ $1 == "logs" ]]; then
    path=http://localhost:8000/rest/v1/logs/$unitname
    curl $path
else
    echo "unknown command"
    exit 1
fi

echo "OK"
exit 0
