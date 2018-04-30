#!/bin/bash

set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <ip> <operation>"
    exit 1
fi

ip=$1
#unitname="helloserver"

if [[ $2 == "logs" ]]; then
    path=http://$ip:8000/rest/v1/logs/foounit
    curl $path
elif [[ $2 == "status" ]]; then
    path=http://$ip:8000/rest/v1/status
    curl $path
elif [[ $2 == "update" ]]; then
    echo "update"
    path=http://$ip:8000/rest/v1/updatepod
    data='{"spec":{"phase":"","units":[{"name":"foounit","image":"elotl/helloserver","command":"/helloserver","env":null,"Ports":null}],"imagePullSecrets":null,"instanceType":"","bootImageTags":null,"restartPolicy":"Always","spot":{"policy":""},"resources":{"cpu":0,"burstable":false,"memory":"","volumeSize":0,"gpu":0}}}'
    # data='{"spec":{"phase":"","units":[],"imagePullSecrets":null,"instanceType":"","bootImageTags":null,"restartPolicy":"Always","spot":{"policy":""},"resources":{"cpu":0,"burstable":false,"memory":"","volumeSize":0,"gpu":0}}}'

    curl -X POST -d "$data" $path
else
    echo "unknown command"
    exit 1
fi

echo "OK"
exit 0
