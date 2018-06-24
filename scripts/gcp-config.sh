#!/bin/bash -e
set -x # For debugging

# Assume we're on a non-EOL version of Ubuntu
GCLOUD_SDK_SOURCES="google-cloud-sdk.list"

# Install gcloud sdk
export CLOUD_SDK_REPO="cloud-sdk-$(lsb_release -c -s)" && \
    echo "deb http://packages.cloud.google.com/apt $CLOUD_SDK_REPO main" | \
    sudo tee  "/etc/apt/sources.list.d/$GCLOUD_SDK_SOURCES" && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | \
    sudo apt-key add - && \
    sudo apt-get update -y && sudo apt-get install google-cloud-sdk -y

# Set gcloud config (equivalent to setting AWS credentials)
GCP_REGION="us-east1"
GCP_ZONE="us-east1-b"
GCP_ACCOUNT="rg2023@caa.columbia.edu"
GCP_PROJECT="myechuri-project1"

gcloud config set core/account $GCP_ACCOUNT
gcloud config set core/project $GCP_PROJECT
gcloud config set compute/region $GCP_REGION
gcloud config set compute/zone $GCP_ZONE

# Authenticate unless --skip-auth flag is set. Once a user does this
# to allow GCP to be accessed by their account they should be able to
# skip it
