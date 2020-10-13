module github.com/elotl/itzo

go 1.13

require (
	cloud.google.com/go v0.56.0
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6
	github.com/aws/aws-sdk-go v1.28.2
	github.com/containerd/cgroups v0.0.0-20190919134610-bf292b21730f
	github.com/containers/buildah v1.16.4
	github.com/containers/image/v5 v5.6.0
	github.com/containers/podman/v2 v2.1.1
	github.com/containers/storage v1.23.5
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e
	github.com/cri-o/ocicni v0.2.0
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/go-units v0.4.0
	github.com/elotl/wsstream v0.0.0-20180531183345-a88a26dd5a78
	github.com/go-ole/go-ole v1.2.4
	github.com/gogo/protobuf v1.3.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/gofuzz v1.1.0
	github.com/gorilla/websocket v1.4.0
	github.com/hashicorp/errwrap v1.0.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/jandre/procfs v0.0.0-20150609131925-f645421657bb
	github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af
	github.com/justnoise/genny v0.0.0-20170328200008-9127e812e1e9
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/kr/pty v1.1.5
	github.com/lorenzosaino/go-sysctl v0.1.0
	github.com/mitchellh/go-ps v1.0.0
	github.com/opencontainers/runc v1.0.0-rc91.0.20200708210054-ce54a9d4d79b
	github.com/opencontainers/runtime-spec v1.0.3-0.20200817204227-f9c09b4ea1df
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/ramr/go-reaper v0.2.0
	github.com/shirou/gopsutil v0.0.0-20190323131628-2cbc9195c892
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/vishvananda/netlink v1.1.0
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df
	golang.org/x/mod v0.2.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/sys v0.0.0-20200909081042-eff7692f9009
	golang.org/x/text v0.3.3
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.18.4
	k8s.io/apimachinery v0.19.2
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.4
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
)

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.4

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.4

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.4

replace k8s.io/apiserver => k8s.io/apiserver v0.18.4

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.4

replace k8s.io/cri-api => k8s.io/cri-api v0.18.4

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.4

replace k8s.io/kubelet => k8s.io/kubelet v0.18.4

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.4

replace k8s.io/apimachinery => k8s.io/apimachinery v0.18.4

replace k8s.io/api => k8s.io/api v0.18.4

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.4

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.4

replace k8s.io/component-base => k8s.io/component-base v0.18.4

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.4

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.4

replace k8s.io/metrics => k8s.io/metrics v0.18.4

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.4

replace k8s.io/code-generator => k8s.io/code-generator v0.18.4

replace k8s.io/client-go => k8s.io/client-go v0.18.4

replace k8s.io/kubectl => k8s.io/kubectl v0.18.4
