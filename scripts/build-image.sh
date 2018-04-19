#!/bin/bash -e

# Defaults.
IMAGE="alpine.qcow2"
IMAGE_SIZE="2G"
NO_AMI=false
ENVIRONMENT="dev"

while [[ -n "$1" ]]; do
    case "$1" in
        "-h"|"--help")
            echo "Usage:"
            echo "    $0 [-h|--help] [-s|--size image size] " \
                "[-o|--out image path] [-n|--no-ami]"
            echo "    -s|--size <size>: image size, default is 2G"
            echo "    -o|--out <output path>: image output path, " \
                "default is alpine.qcow2"
            echo "    -n|--no-ami: don't create AMI from qcow2 image"
            echo "Example:"
            echo "    $0 -o my-alpine-image.qcow2 -s 2G"
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
        "-n"|"--no-ami")
            shift
            NO_AMI=true
            ;;
	"-e"|"--environment")
	    shift
	    ENVIRONMENT="$1"
            if [[ $ENVIRONMENT != "dev" ]] && [[ $ENVIRONMENT != "prod" ]]
                echo "Error, invalid environment specified."
                exit 1
            fi
            shift
            ;;
        *)
            echo "Error, invalid argument $1"
            exit 1
            ;;
    esac
done

IMAGE_ABSPATH="$(readlink -f $IMAGE)"

REQUIRED_PACKAGES="qemu-utils python-boto3"
echo "Checking if required packages are installed."
for pkg in $REQUIRED_PACKAGES; do
    echo -n "$pkg..."
    if dpkg -l|grep '^ii'|awk '{print $2}'|grep "$pkg" > /dev/null 2>&1; then
        echo "OK"
    else
        echo "MISSING"
        exit 1
    fi
done

if [ "$NO_AMI" = false ]; then
    if [[ -z "$AWS_ACCESS_KEY_ID" ]] || [[ -z "$AWS_SECRET_ACCESS_KEY" ]]; then
        echo "Error: please set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY."
        exit 1
    fi
fi

if [[ $EUID -ne 0 ]]; then
    echo "Warning: not running as root, certain operations might fail."
    echo "Please retry as root if something fails:"
    echo "    sudo AWS_ACCESS_KEY_ID=\$AWS_ACCESS_KEY_ID " \
        "AWS_SECRET_ACCESS_KEY=\$AWS_SECRET_ACCESS_KEY $@"
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
./alpine-make-vm-image --kernel-flavor vanilla --image-format qcow2 --image-size "$IMAGE_SIZE" --repositories-file ../elotl/repositories --keys-dir ../elotl/keys --packages "$(cat ../elotl/packages)" --script-chroot "$IMAGE_ABSPATH" -- ../elotl/configure.sh --environment $ENVIRONMENT

popd > /dev/null

if $NO_AMI; then
    exit 0
fi

AMI_NAME=alpine-$(date +%s)
if [ $ENVIRONMENT != "prod" ]; then
    AMI_NAME=$AMI_NAME-$ENVIRONMENT
fi
python ec2-make-ami.py --input "$IMAGE_ABSPATH" --name $AMI_NAME
