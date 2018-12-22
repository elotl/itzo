#!/bin/bash -e

# Defaults.
IMAGE="alpine.qcow2"
IMAGE_SIZE="2G"
NO_IMAGE=false
BUILD_VERSION=""

while [[ -n "$1" ]]; do
    case "$1" in
        "-h"|"--help")
            echo "Usage:"
            echo "    $0 [-h|--help] [-s|--size image size] " \
                "[-o|--out image path] [-n|--no-image]"
            echo "    -s|--size <size>: image size, default is 2G"
            echo "    -o|--out <output path>: image output path, " \
                "default is alpine.qcow2"
            echo "    -n|--no-image: don't create Image from qcow2 image"
            echo "    -v|--version <buildnumber>: version of the image, used in image name" \
            echo "Example:"
            echo "    $0 -o my-alpine-image.qcow2 -s 2G -e prod -v 16"
            exit 0
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

IMAGE_ABSPATH="$(readlink -f $IMAGE)"

REQUIRED_PROGRAMS="qemu-img qemu-nbd"
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
found=true
# echo -n "boto3..."
# python -c "import boto3" > /dev/null 2>&1 || found=false
# if [[ $found = false ]]; then
#    echo "MISSING boto3"
#    exit 1
# fi
# echo "OK"

if [ "$NO_IMAGE" = false ]; then
    if [ -z "$GCE_SERVICE_ACCOUNT_FILE" ]; then
        echo "Error: please set GCE_SERVICE_ACCOUNT_FILE."
        exit 1
    fi
fi

if [[ $EUID -ne 0 ]]; then
    echo "Warning: not running as root, certain operations might fail."
    echo "Please retry as root"
    exit 0
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

git checkout 86e334
git clean -fdx

#
# You can test the image locally via something like this:
#
#     kvm -m 512 -net nic,model=virtio -net user,hostfwd=tcp:127.0.0.1:9222-:22 -drive file=alpine.qcow2,if=virtio
#
./alpine-make-vm-image --kernel-flavor vanilla --image-format qcow2 --image-size "$IMAGE_SIZE" --repositories-file ../elotl/repositories --keys-dir ../elotl/keys --packages "$(cat ../elotl/packages)" --script-chroot "$IMAGE_ABSPATH" -- ../elotl/configure.sh

popd > /dev/null

if $NO_IMAGE; then
    exit 0
fi

PRODUCT_NAME="milpadev"
IMAGE_NAME=elotl-$PRODUCT_NAME-$BUILD_VERSION-$(date +"%Y%m%d-%H%M%S")
python gce-make-image.py --input "$IMAGE_ABSPATH" --name $IMAGE_NAME
