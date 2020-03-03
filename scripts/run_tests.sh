#!/bin/bash

# Copyright 2020 Elotl Inc
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..

cd $ROOT_DIR
go test ./...

CURRENT_BRANCH=$(echo $GIT_BRANCH | sed -e "s|origin/||g")
CURRENT_TAG=$(git tag -l --points-at HEAD | head -n 1)

echo "Current branch is $CURRENT_BRANCH"
echo "Current tag is $CURRENT_TAG"
echo "Build number is $BUILD_NUMBER"

if [[ $CURRENT_TAG =~ ^v[0-9].* ]] || [[ $CURRENT_BRANCH == "master" ]]; then
    echo "Building itzo binary"
    export PATH=$PATH:$HOME/.local/bin
    make itzo
    itzo_build="itzo-$BUILD_NUMBER"
    itzo_dev_bucket=itzo-dev-download
    aws s3 cp itzo s3://$itzo_dev_bucket/$itzo_build --acl public-read
    aws s3 cp itzo s3://$itzo_dev_bucket/itzo-latest --acl public-read
    if [[ $CURRENT_TAG =~ ^v[0-9].* ]]; then
	versiontag=$CURRENT_TAG
	echo "Making an itzo release at $versiontag"
	echo "How can you say you wanna sit there With all this funk going on?"
	echo "Get up. Time to release the beast!"
	release_file=itzo-$versiontag
	release_bucket=itzo-download
	aws s3 cp itzo s3://$release_bucket/$release_file --acl public-read
	aws s3 cp itzo s3://$release_bucket/itzo-latest --acl public-read
    fi
fi
