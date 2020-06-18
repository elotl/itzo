# Itzo

Itzo is the agent that runs on [Kip](https://github.com/elotl/kip) cells (cloud instances) and takes care of managing the lifecycle of pods and containers on the instance.  In many ways, itzo performs many of the duties of the kubelet:

* Downloading containers
* Setting up namespaces
* Running and restarting processes
* Running probes
* Mounting volumes
* Capturing logs

## Creating Images

Scripts and a packer config building cell images on AWS and GCP can be found [here](https://github.com/elotl/kip-cell-image).
