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

### Setting up Xcode

Download the latest versions of Xcode and Command Line Tools for Xcode from
https://developer.apple.com/download/more/

You’ll need to be logged in with your Apple ID to be able to download the
binaries. There are big: 10+GB for both of them. Once the archives are
downloaded install them on the host.

First the Command line tools:

    $ hdiutil attach Command_Line_Tools_for_Xcode_12.3.dmg
    $ cd /Volumes/Command\ Line\ Developer\ Tools/
    $ sudo installer -pkg Command\ Line\ Tools.pkg -target /
    ...
    installer: The upgrade was successful.

Second Xcode itself:

    $ pkgutil --check-signature Xcode_12.3.xip |
        grep \"Status: signed Apple Software\"
    # Clean up existing Xcode installation if needed
    $ rm -rf /Applications/Xcode.app
    # Install Xcode from XIP file Location
    $ (cd /Applications; xip --expand /Users/wc2-user/Xcode_12.3.xip)
    # Accept License Agreement
    /Applications/Xcode.app/Contents/Developer/usr/bin/xcodebuild -license accept
    /Applications/Xcode.app -license accept
    # Run Xcode first launch
    /Applications/Xcode.app -runFirstLaunch

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
