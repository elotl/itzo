# Itzo

Itzo is the agent that runs on kip cells (cloud instances) and takes care of running containers on the instance.  In many ways, itzo performs many of the duties of the kubelet:

* Downloading containers
* Setting up namespaces
* Running and restarting processes
* Running probes
* Mounting volumes
* Capturing logs

## Creating Images

Scripts in this repo can be used to create the Alpine linux machine images that are used as the root disk by kip cells.  When the image boots it will download the itzo binary via http.  By default the image is downloaded from http://itzo-download.s3.amazonaws.com.  That location is configurable in the [cloud-instance-provider config](https://github.com/elotl/cloud-instance-provider/blob/master/provider-config.md).
