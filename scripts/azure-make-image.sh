#!/bin/bash

set -e

# az storage account create --resource-group $RESOURCE_GROUP --location $LOCATION --name $STORAGE_ACCOUNT --kind Storage --sku Standard_LRS
# az storage container create --account-name $STORAGE_ACCOUNT --name $STORAGE_CONTAINER
# az storage account keys list --resource-group $RESOURCE_GROUP --account-name $STORAGE_ACCOUNT

# Args:
# input image name
# name of image in azure
if [ $# -lt 2 ]; then
    script_name=`basename $0`
    echo "Usage: $script_name <input_image_path> <cloud_image_name>"
    exit 1
fi

IMAGE_ABSPATH="$1"
IMAGE_NAME="$2"

# Constants
LOCATION="West US 2"
RESOURCE_GROUP="elotl-resources"
STORAGE_ACCOUNT="elotlimages"
STORAGE_CONTAINER="itzodisks"

echo "building from $IMAGE_ABSPATH into $IMAGE_NAME"

az login --service-principal --tenant=$AZURE_TENANT_ID --username=$AZURE_CLIENT_ID --password=$AZURE_CLIENT_SECRET

qemu-img convert -f qcow2 -O raw $IMAGE_ABSPATH alpine.img
MB=$((1024 * 1024))
SIZE=$(qemu-img info -f raw --output json alpine.img |  gawk 'match($0, /"virtual-size": ([0-9]+),/, val) {print val[1]}')
ROUNDED_SIZE=$((($SIZE/$MB + 1) * $MB))
echo "resizing to $ROUNDED_SIZE"
qemu-img resize -f raw alpine.img $ROUNDED_SIZE
qemu-img convert -f raw -O vpc -o subformat=fixed,force_size alpine.img alpine.vhd

STORAGE_KEY=$(az storage account keys list --resource-group $RESOURCE_GROUP --account-name $STORAGE_ACCOUNT | jq -r '.[0].value')

az storage blob upload --account-name $STORAGE_ACCOUNT --account-key $STORAGE_KEY --container-name $STORAGE_CONTAINER --type page --file ./alpine.vhd --name ${IMAGE_NAME}.vhd

# note that zone resiliant disks can only be created in
# locations/regions that support Zone Redundant Storage we will likely
# need this to be able to support launching VMs across availability
# zones when an AZ is down
az image create --storage-sku StandardSSD_LRS --zone-resilient true --os-disk-caching ReadWrite --resource-group $RESOURCE_GROUP --os-type=Linux --source https://$STORAGE_ACCOUNT.blob.core.windows.net/$STORAGE_CONTAINER/${IMAGE_NAME}.vhd --name $IMAGE_NAME
