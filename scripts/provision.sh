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

echo "make go think it has a libc"
mkdir $alpine_mnt/lib64
ln -s /lib/libc.musl-x86_64.so.1 $alpine_mnt/lib64/ld-linux-x86-64.so.2

# echo "copying itzo to image"
# cp $DIR/itzo $alpine_mnt/usr/local/bin/itzo
echo "copying itzo downloader-launcher to image"
cp $DIR/itzo_download.sh $alpine_mnt/usr/local/bin/itzo_download.sh
echo "setup itzo init scripts"
cp $DIR/itzo.rc $alpine_mnt/etc/init.d/itzo
ln -s /etc/init.d/itzo $alpine_mnt/etc/runlevels/default/itzo

echo "Unmounting alpine partition"
umount $alpine_mnt
