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

set -o errexit

# Args:
if [ $# -lt 2 ]; then
    script_name=`basename $0`
    echo "Usage: $script_name <input_image_path> <cloud_image_name>"
    exit 1
fi

IMAGE_ABSPATH="$1"
IMAGE_NAME="$2"

# Constants
readonly STORAGE_CONTAINER="itzodisks"
readonly GCE_IMAGE_NAME="disk.raw"
readonly GCE_TAR_NAME="$IMAGE_NAME.tar.gz"

echo "building from $IMAGE_ABSPATH into $IMAGE_NAME"

echo "converting qcow2 to raw"
qemu-img convert -f qcow2 -O raw "$IMAGE_ABSPATH" "$GCE_IMAGE_NAME"
echo "creating compressed tar archive"
sudo tar --format=oldgnu -Sczf "$GCE_TAR_NAME" "$GCE_IMAGE_NAME"
echo "creating bucket $STORAGE_CONTAINER if it does not exist"
gsutil mb gs://"$STORAGE_CONTAINER" || echo "bucket exists"
echo "copying compressed image to bucket: $STORAGE_CONTAINER"
gsutil cp "$GCE_TAR_NAME" gs://"$STORAGE_CONTAINER"
echo "creating image for GCE compute images"
gcloud compute images create "$IMAGE_NAME" --source-uri gs://"$STORAGE_CONTAINER/$GCE_TAR_NAME"

