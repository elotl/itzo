#!/bin/bash

set -e

if [ "$EUID" -ne 0 ]
  then echo "Provisioning must be run as root"
  exit
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

alpine_mnt=$DIR/mount
if [ ! -d $alpine_mnt ]; then
    mkdir $alpine_mnt
fi
if mount | grep $alpine_mnt > /dev/null; then
    echo "volume is already mounted"
else
    echo "Mounting image to $alpine_mnt"
    mount /dev/xvdf3 $alpine_mnt
fi

sshdir=$alpine_mnt/root/.ssh
if [ ! -d  $sshdir ]; then
    echo "making .ssh dir"
    mkdir $sshdir
fi
cp $HOME/.ssh/authorized_keys ${sshdir}/authorized_keys
chmod 600 $sshdir

# we should be downloading our release of itzo from an s3 bucket after
# it gets built by our CI system but we ain't go that yet
# for now, we'll assume it's in the same directory
# cd $DIR/../itzo/
# go build
echo "copying itzo to image"
cp $DIR/itzo $alpine_mnt/usr/bin/itzo
echo "setup itzo init scripts"
cp $DIR/itzo.rc $alpine_mnt/etc/init.d/itzo
ln -s /etc/init.d/itzo $alpine_mnt/etc/runlevels/default/itzo

echo "Unmounting alpine partition"
umount $alpine_mnt
