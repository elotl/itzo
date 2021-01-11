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
