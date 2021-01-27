# Provisioning AWS Mac bare metal instances for Itzo

This document outlines what necessary steps go into provisioning an AWS bare metal Mac instance

### Resize the disk on the instance

By default when AWS stands up a mac bare metal instance it only allocates 32GiB \
of storage to the disk you are interacting with no matter how large the EBS volume 
you requested was.

**Steps**

```bash
PDISK=$(diskutil list physical external | head -n1 | cut -d' ' -f1)
APFSCONT=$(diskutil list physical external | grep Apple_APFS | tr -s ' ' | cut -d' ' -f8)

sudo diskutil repairDisk $PDISK
sudo diskutil apfs resizeContainer $APFSCONT 0
```

### Installing Anka

Veertu provides a repository called `getting-started` which can be found [here](https://github.com/veertuinc/getting-started).

We also need to obtain a license for the `Anka Build` product and that can be done [here](https://veertu.com/getting-started-anka-trials/).

**Steps**

```bash
# First connect to the new mac instance via ssh or ssm; however you prefer

git clone https://github.com/veertuinc/getting-started.git
cd getting-started

# when running this script you will be asked for the API Key we requested for Anka Build
./install-anka-virtualization-on-mac.bash

# this will install the Anka controller and registry
./ANKA_BUILD_CLOUD/install-anka-build-controller-and-registry-on-mac.bash

# this will generate the vm template
./create-vm-template.bash
```

### Setting up Xcode on the anka VM

Download the latest versions of Xcode and Command Line Tools for Xcode from
https://developer.apple.com/download/more/ . Command line tools for Xcode
aren’t enough. One must install the full Xcode.


You’ll need to be logged in with your Apple ID to be able to download the
binaries. There are big: 10+GB for both of them. NOTE: there are ways to
generate a cookie that developer.apple.com will accept. If you try it out
please update this document.

Once the archives are downloaded install them on the VM. You can either copy
the Xcode package to the VM with `sudo anka cp VMID XcodeXX.X.xip` or mount
the directory where it is located on the host with `sudo anka mount dir/`.

All these commands should be run through `sudo anka run VMID ...`:

    $ pkgutil --check-signature Xcode_12.3.xip |
        grep \"Status: signed Apple Software\"
    # Clean up existing Xcode installation if needed
    $ rm -rf /Applications/Xcode.app
    # Install Xcode from XIP file Location
    $ (cd /Applications; xip --expand /Users/wc2-user/Xcode_12.3.xip)
    # Accept License Agreement
    /Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild -license accept
    # Run Xcode first launch
    /Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild -runFirstLaunch
    # Make sure to use the right toolset
    sudo xcode-select -s /Applications/Xcode.app/Contents/Developer

There’s a Ruby Gem to install xcode: https://github.com/xcpretty/xcode-install

It’s broken out of the box T_T with Xcode 12. Luckily these aren’t
Apple devs, and they made it easy to install despite Apple’s shenanigans:
https://github.com/xcpretty/xcode-install#installation

I installed Xcode 11 like that:

    $ env XCODE_INSTALL_USER=*****@elotl.co XCODE_INSTALL_PASSWORD='******' xcversion install 11

This will download and unpack the Xcode 11 archive. It takes more than
1 hour to complete. I also had to install the iOS simulators 13.0 and
14.2... Simulators aren’t included in Xcode’s 15GB package...

NOTE: when installing the iOS 13.0 simulator. You will get a message "Please
authenticate to install iOS 13.0 Simulator...", just wait for a few minutes
and the simulator will be installed.

### Creating an AMI

In order to allow us to boot an instance quickly without doing all of the above \
steps we should create a bootable machine image

```bash
aws ec2 create-image \
    --description="description of the image" \
    --instance-id INSTANCE_ID_HERE \
    --name "name-of-the-image" \
    --region us-west-2
```
