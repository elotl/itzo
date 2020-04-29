#!/bin/bash -e

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

# Defaults.
IMAGE="alpine.qcow2"
IMAGE_SIZE="2G"
NO_IMAGE=false
BUILD_VERSION=""
CLOUD_PROVIDER=""

while [[ -n "$1" ]]; do
    case "$1" in
        "-h"|"--help")
            echo "Usage:"
            echo "    $0 [-h|--help] [-s|--size image size] " \
                "[-o|--out image path] [-n|--no-image] [-c|--cloud cloud provider]"
	    echo "    -c|--cloud <provider>: Target cloud (aws|azure)"
            echo "    -s|--size <size>: image size, default is 2G"
            echo "    -o|--out <output path>: image output path, " \
                "default is alpine.qcow2"
            echo "    -n|--no-image: don't create AMI from qcow2 image"

            echo "    -v|--version <buildnumber>: version of the image, used in image name" \
            echo "Example:"
            echo "    $0 -c aws -o my-alpine-image.qcow2 -s 2G -e prod -v 16"
            exit 0
            ;;
        "-c"|"--cloud")
            shift
            CLOUD_PROVIDER="$1"
            if [[ "$CLOUD_PROVIDER" != "aws" ]] && 
                [[ "$CLOUD_PROVIDER" != "azure" ]] &&
                [[ "$CLOUD_PROVIDER" != "gce" ]]; then
                echo "Error, invalid cloud provider specified."
                exit 1
            fi
            shift
            ;;
        "-s"|"--size")
            shift
            IMAGE_SIZE="$1"
            if [[ -z "$IMAGE_SIZE" ]] || [[ ${1:0:1} == "-" ]]; then
                echo "Error, invalid image size specified."
                exit 1
            fi
            shift
            ;;
        "-o"|"--out")
            shift
            IMAGE="$1"
            if [[ -z "$IMAGE" ]] || [[ ${1:0:1} == "-" ]]; then
                echo "Error, invalid image output path specified."
                exit 1
            fi
            shift
            ;;
        "-n"|"--no-image")
            shift
            NO_IMAGE=true
            ;;
	"-v"|"--version")
	    shift
	    BUILD_VERSION="$1"
	    case $BUILD_VERSION in
		''|*[!0-9]*)
		    echo "Version must be an integer"
		    exit 1
	    esac
            shift
            ;;
        *)
            echo "Error, invalid argument $1"
            exit 1
            ;;
    esac
done

if [[ -z "$BUILD_VERSION" ]]; then
    echo "A build version (--version) is required"
    exit 1
fi

if [[ -z "$CLOUD_PROVIDER" ]]; then
    echo "Cloud provider (-c or --cloud) is required"
    exit 1
fi

IMAGE_ABSPATH="$(readlink -f $IMAGE)"

if [[ "$CLOUD_PROVIDER" == "aws" ]] && [[ "$NO_IMAGE" = false ]]; then
    found=true
    echo -n "boto3..."
    python -c "import boto3" > /dev/null 2>&1 || found=false
    if [[ $found = false ]]; then
	echo "MISSING boto3"
	exit 1
    fi
    if [[ -z "$AWS_ACCESS_KEY_ID" ]] || [[ -z "$AWS_SECRET_ACCESS_KEY" ]]; then
        echo "Error: please set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY."
        exit 1
    fi
    echo "OK"
fi

if [[ "$CLOUD_PROVIDER" == "azure" ]] && [[ "$NO_IMAGE" = false ]]; then
    REQUIRED_PROGRAMS="$REQUIRED_PROGRAMS az jq"
    if [[ -z "$AZURE_TENANT_ID" ]] || [[ -z "$AZURE_CLIENT_ID" ]] || [[ -z "$AZURE_CLIENT_SECRET" ]]; then
        echo "Error: please set AZURE_TENANT_ID, AZURE_CLIENT_ID and AZURE_CLIENT_SECRET."
        exit 1
    fi
    echo "OK"
fi

if [[ "$CLOUD_PROVIDER" == "gce" ]] && [[ "$NO_IMAGE" = false ]]; then
    REQUIRED_PROGRAMS="$REQUIRED_PROGRAMS gsutil gcloud jq"
    if [[ ! -d "$HOME/.config/gcloud" ]]; then
        echo "Error: please set up gcloud with a service account"
        echo "e.g. gcloud auth activate-service-account --key-file PATH_TO_KEYFILE.json"
        exit 1
    fi
    echo "OK"
fi

REQUIRED_PROGRAMS="$REQUIRED_PROGRAMS qemu-img qemu-nbd"
echo "Checking if required programs are installed."
for prg in $REQUIRED_PROGRAMS; do
    found=true
    echo -n "$prg..."
    which $prg > /dev/null 2>&1 || found=false
    if [[ $found = false ]]; then
        echo "MISSING $prg"
        exit 1
    fi
    echo "OK"
done

if [[ $EUID -ne 0 ]]; then
    echo "Warning: not running as root, certain operations might fail."
    echo "Please retry as root if something fails:"
    echo "    sudo $@"
fi

if [[ -f "$IMAGE" ]]; then
    echo "Warning: $IMAGE already exists."
    echo "Press CTRL-C to abort, or Enter to continue and overwrite it."
    read
    rm -f "$IMAGE"
fi

echo "Creating image of size $IMAGE_SIZE at $IMAGE_ABSPATH."

[[ -f ./alpine-make-vm-image/.git/HEAD ]] || git clone --recurse-submodules https://github.com/alpinelinux/alpine-make-vm-image.git

pushd alpine-make-vm-image > /dev/null

git checkout ea6dcfe63580dc3c4aa14c7bb362c9bb67f23e01
git clean -fdx

#
# You can test the image locally via something like this:
#
#     kvm -m 512 -net nic,model=virtio -net user,hostfwd=tcp:127.0.0.1:9222-:22 -drive file=alpine.qcow2,if=virtio
#
PACKAGES="$(cat ../elotl/packages-$CLOUD_PROVIDER)"
./alpine-make-vm-image --kernel-flavor virt --image-format qcow2 --image-size "$IMAGE_SIZE" --repositories-file ../elotl/repositories --keys-dir ../elotl/keys --packages "$PACKAGES" --script-chroot "$IMAGE_ABSPATH" -- ../elotl/configure.sh "$CLOUD_PROVIDER"

popd > /dev/null

if $NO_IMAGE; then
    exit 0
fi

PRODUCT_NAME="kipdev"
IMAGE_NAME=elotl-$PRODUCT_NAME-$BUILD_VERSION-$(date +"%Y%m%d-%H%M%S")

if [[ "$CLOUD_PROVIDER" == "aws" ]]; then
    python ec2-make-ami.py --input "$IMAGE_ABSPATH" --name $IMAGE_NAME
elif [[ "$CLOUD_PROVIDER" == "azure" ]]; then
    ./azure-make-image.sh "$IMAGE_ABSPATH" $IMAGE_NAME
elif [[ "$CLOUD_PROVIDER" == "gce" ]]; then
    ./gce-make-image.sh "$IMAGE_ABSPATH" "$IMAGE_NAME"
else
    echo "Unknown cloud provider: $CLOUD_PROVIDER"
fi
