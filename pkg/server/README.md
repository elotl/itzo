## Running podman E2E tests locally
You can find tests in [server_podman_test.go](./server_podman_test.go). 
Test framework allows you to safely test itzo (with podman runtime) locally. It should massively speed up development, as it means that you no longer need to spin up whole cluster + kip + actual VM with itzo.  
Before running those you need to:
1. Ensure that podman.socket is open for superuser by running sudo systemctl start podman.socket (this will create socket in `/run/podman/podman.socket`)
2. Ensure that sudo podman pod ps doesn't return itzpod - that's a constant that we use for pod created by itzo.
3. Executing of those test may take longer as they're E2E; using podman API we create, run, stop and remove pods and containers here.
All podman resources should be removed after each test by removeContainersAndPods function.

Those tests don't run by default. To run them, you need to set podman flag to true, e.g.: `sudo go test ./pkg/server/   -v -args -podman=true`. You need to run it as superuser, because itzo expects podman socket in `/run/podman/podman.socket` (for now it isn't configurable)
