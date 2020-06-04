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

PATH=$PATH:$HOME/.local/bin
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
ROOT_DIR=$SCRIPT_DIR/..

cd $ROOT_DIR
make
go test ./...

CURRENT_TAG=$(git tag -l --points-at HEAD | head -n 1)

CURRENT_BRANCH=$(echo $TRAVIS_PULL_REQUEST_BRANCH | sed -e "s|origin/||g")
if [[ -z "$CURRENT_BRANCH" ]]; then
    CURRENT_BRANCH=$(echo $TRAVIS_BRANCH | sed -e "s|origin/||g")
fi
if [[ -z "$CURRENT_BRANCH" ]]; then
    echo "Error: failed to detect current branch."
    exit 1
fi

CURRENT_BUILD_NUMBER="$TRAVIS_BUILD_NUMBER"
if [[ -z "$CURRENT_BUILD_NUMBER" ]]; then
    echo "Error: failed to detect current build number."
    exit 1
fi

echo "Current tag is \"$CURRENT_TAG\""
echo "Current branch is \"$CURRENT_BRANCH\""
echo "Build number is \"$CURRENT_BUILD_NUMBER\""

itzo_release=false
itzo_bucket="itzo-kip-download"
itzo_dev_bucket="itzo-kip-dev-download"
if [[ $CURRENT_TAG =~ ^v[0-9].* ]]; then
    itzo_release=true
fi

#
# We use two buckets: itzo-kip-dev-download for builds, and itzo-kip-download
# for releases (tagged using a semantic version, in the form of vX.Y.Z).
#
# We upload each build to itzo-kip-dev-download, and update itzo-latest if this
# is the master branch.
#
# If the commit is tagged with a release version (vX.Y.Z), we also update the
# build to itzo-kip-download, and update itzo-latest there.
#
echo "Uploading itzo build $CURRENT_BUILD_NUMBER"
aws s3 cp --acl public-read itzo s3://$itzo_dev_bucket/itzo-$CURRENT_BUILD_NUMBER
gsutil copy itzo gs://$itzo_dev_bucket/itzo-$CURRENT_BUILD_NUMBER && \
    gsutil acl ch -u AllUsers:R gs://$itzo_dev_bucket/itzo-$CURRENT_BUILD_NUMBER
if [[ $CURRENT_BRANCH == "master" ]]; then
	aws s3 cp --acl public-read itzo s3://$itzo_dev_bucket/itzo-latest
    gsutil copy itzo gs://$itzo_dev_bucket/itzo-$CURRENT_BUILD_NUMBER && \
        gsutil acl ch -u AllUsers:R gs://$itzo_dev_bucket/itzo-$CURRENT_BUILD_NUMBER
fi
if $itzo_release; then
    echo "Making an itzo release at $CURRENT_TAG"
	aws s3 cp --acl public-read itzo s3://$itzo_bucket/itzo-$CURRENT_TAG
    gsutil copy itzo gs://$itzo_bucket/itzo-$CURRENT_TAG && \
        gsutil acl ch -u AllUsers:R gs://$itzo_bucket/itzo-$CURRENT_TAG
	aws s3 cp --acl public-read itzo s3://$itzo_bucket/itzo-latest
    gsutil copy itzo gs://$itzo_bucket/itzo-latest && \
        gsutil acl ch -u AllUsers:R gs://$itzo_bucket/itzo-latest
fi
