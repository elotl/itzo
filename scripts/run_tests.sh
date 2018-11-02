#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..

cd $ROOT_DIR
go test ./...

# if our travis tag looks like a version and we're on a tagged branch
versiontag=""
if [[ $TRAVIS_TAG =~ ^v[0-9].* ]] && [[ $TRAVIS_TAG == $TRAVIS_BRANCH ]]; then
    echo "How can you say you wanna sit there With all this funk going on?"
    echo "Get up. Time to release the beast!"
    export PATH=$PATH:$HOME/.local/bin
    versiontag=$TRAVIS_BRANCH
    make itzo
    echo "Making an itzo release at $versiontag"
    release_file=itzo-$versiontag
    release_bucket=itzo-download
    aws s3 cp itzo s3://$release_bucket/$release_file --acl public-read
    aws s3 cp itzo s3://$release_bucket/itzo-latest --acl public-read
fi
