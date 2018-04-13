package api

import (
	"strings"
)

type PodSpec struct {
	// Desired condition of the pod.
	Phase PodPhase `json:"phase"`
	// List of units that together compose this pod.
	Units []Unit `json:"units"`
	// List of secrets that will be used for authenticating when pulling
	// images.
	ImagePullSecrets []string `json:"imagePullSecrets,omitemtpy"`
	// Type of cloud instance type that will be used to run this pod.
	InstanceType string `json:"instanceType"`
	// Dictionary of image tags to determine which cloud image should be used
	// to run this pod.  The latest available image that satisfies all these
	// tags will be used to run this pod.
	//
	// Example:
	//
	// ```
	// bootImageTags:
	//   environment: production
	//   version: 1.2.3
	// ```
	BootImageTags map[string]string `json:"bootImageTagso"`
	// Restart policy for all units in this pod. It can be "always",
	// "onFailure" or "never". Default is "always".
	RestartPolicy RestartPolicy `json:"restartPolicy"`
	// PodSpot is the policy that determines if a spot instance may be used for
	// a pod.
	Spot PodSpot `json:"spot,omitempty"`
	// Resource requirements for the node that will run this pod. If both
	// instanceType and resources are specified, instanceType will take
	// precedence.
	Resources ResourceSpec `json:"resources,omitempty"`
	// List of volumes that will be made available to the pod. Units can then
	// attach any of these mounts.
	Volumes []Volume `json:"volumes,omitempty"`
}

// Defintion for volumes.
type Volume struct {
	// Name of the volume. This is used when referencing a volume from a unit
	// definition.
	Name         string `json:"name"`
	VolumeSource `json:",inline,omitempty"`
}

type VolumeSource struct {
	// If specified, an emptyDir will be created to back this volume.
	EmptyDir *EmptyDir `json:"emptyDir,omitempty"`
}

// Backing storage for volumes.
type StorageMedium string

const (
	StorageMediumDefault StorageMedium = ""       // Use default (disk).
	StorageMediumMemory  StorageMedium = "Memory" // Use tmpfs.
	// Supporting huge pages will require some extra steps.
	//StorageMediumHugePages StorageMedium = "HugePages" // use hugepages
)

// EmptyDir is is disk or memory-backed volume. Units can use it as
// scratch space, or for inter-unit communication (e.g. one unit
// fetching files into an emptyDir, another running a webserver,
// serving these static files from the emptyDir).
type EmptyDir struct {
	// Backing medium for the emptyDir. The default is "" (to use disk
	// space).  The other option is "Memory", for creating a tmpfs
	// volume.
	Medium StorageMedium `json:"medium,omitempty"`
	// SizeLimit is only meaningful for tmpfs. It is the size of the tmpfs
	// volume.
	SizeLimit int64 `json:"sizeLimit,omitempty"`
}

// ResourceSpec is used to specify resource requirements for the node
// that will run a pod.
type ResourceSpec struct {
	// The number of cpus on the instance, can be a float for
	// burstable instances.
	CPU float32 `json:"cpu"`
	// Request an instance with burstable CPU. For AWS this is a T
	// instance with a variable speed CPU.
	Burstable bool `json:"burstable"`
	// The amount of memory on the instance in gigabytes. For AWS this
	// is in GB and for GCE this is in GiB.
	Memory string `json:"memory"`
	// Root volume size in GB. All units will share this disk.
	VolumeSize int64 `json:"volumeSize"`
	// Number of GPUs present on the instance.
	GPU int `json:"gpu"`
}

// Units run applications. A pod consists of one or more units.
type Unit struct {
	// Name of the unit.
	Name string `json:"name"`
	// The docker image that will be pulled for this unit. Usual docker
	// conventions are used to specify an image, see
	// **[https://docs.docker.com/engine/reference/commandline/tag/#extended-description](https://docs.docker.com/engine/reference/commandline/tag/#extended-description)**
	// for a detailed explanation on specifying an image.
	//
	// Examples:
	//
	// - `library/python:3.6-alpine`
	//
	// - `myregistry.local:5000/testing/test-image`
	//
	Image string `json:"image"`
	// The command that will be run to start the unit.
	Command string `json:"command"`
	// List of environment variables that will be exported inside the unit
	// before start the application.
	Env []EnvVar `json:"env"`
	// A list of volumes that will be attached in the unit.
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`
	// A list of ports that will be opened up for this unit.
	Ports []ServicePort
}

// VolumeMount specifies what volumes to attach to the unit and the path where
// they will be located inside the unit.
type VolumeMount struct {
	// Name of the volume to attach.
	Name string `json:"name"`
	// Path where this volume will be attached inside the unit.
	MountPath string `json:"mountPath"`
}

// Environment variables.
type EnvVar struct {
	// Name of the environment variable.
	Name string `json:"name"`
	// Value of the environment variable.
	Value string `json:"value"`
	// An environment variable may also come from a secret.
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty"`
}

// EnvVarSource represents a source for the value of an EnvVar. Only one of its
// fields may be set.
type EnvVarSource struct {
	// Selector for the secret.
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// SecretKeySelector selects a key of a Secret.
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	Name string `json:"name"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key"`
	// k8s allows optional secrets.  We can add that soon
	// Optional *bool
}

// Spot policy. Can be "always", "preferred" or "never", meaning to always use
// a spot instance, use one when available, or never use a spot instance for
// running a pod.
type SpotPolicy string

const (
	SpotAlways    SpotPolicy = "Always"
	SpotPreferred SpotPolicy = "Preferred"
	SpotNever     SpotPolicy = "Never"
)

// PodSpot is the policy that determines if a spot instance may be used for a
// pod.
type PodSpot struct {
	// Spot policy. Can be "always", "preferred" or "never", meaning to always
	// use a spot instance, use one when available, or never use a spot
	// instance for running a pod.
	Policy SpotPolicy `json:"policy"`
	// Notify string     `json:"notify"`
}

// Phase is the last observed phase of the pod. Can be "creating",
// "dispatching", "running", "succeeded", "failed" or "terminated".
type PodPhase string

const (
	// PodCreating means that we're waiting for the pod to begin running.
	PodCreating PodPhase = "Creating"
	// PodDispatching means that we have a node to put this pod on
	// and we're in the process of starting the app on the node
	PodDispatching PodPhase = "Dispatching"
	// PodRunning means that the pod is up and running.
	PodRunning PodPhase = "Running"
	// Pod succeeded means all the units in the pod returned success. It is a
	// terminal phase, i.e. the final phase when a pod finished. Once the pod
	// finished, Spec.Phase and Status.Phase are the same.
	PodSucceeded PodPhase = "Succeeded"
	// Pod has failed, either a unit failed, or some other problem occurred
	// (e.g. dispatch error). This is a terminal phase.
	PodFailed PodPhase = "Failed"
	// PodTerminated means that the pod has stopped by request. It is a
	// terminal phase.
	PodTerminated PodPhase = "Terminated"
)

// Restart policy for all units in this pod. It can be "always", "onFailure" or
// "never". Default is "always".
type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "Always"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
	RestartPolicyNever     RestartPolicy = "Never"
)

// Service port definition. This is a TCP or UDP port that a service uses.
type ServicePort struct {
	// Name of the service port.
	Name string `json:"name"`
	// Protocol. Can be "TCP", "UDP" or "ICMP".
	Protocol Protocol `json:"protocol"`
	// Port number. Not used for "ICMP".
	Port int `json:"port"`
}

// Protocol defines network protocols supported for things like ports.
type Protocol string

func MakeProtocol(p string) Protocol {
	return Protocol(strings.ToUpper(p))
}

const (
	ProtocolTCP  Protocol = "TCP"
	ProtocolUDP  Protocol = "UDP"
	ProtocolICMP Protocol = "ICMP"
)

type UnitStateWaiting struct {
	Reason        string
	LaunchFailure bool
}

type UnitStateRunning struct {
	StartedAt Time `json:"startedAt,omitempty"`
}

type UnitStateTerminated struct {
	ExitCode   int32 `json:"exitCode"`
	FinishedAt Time  `json:"finishedAt,omitempty"`
}

// UnitState holds a possible state of a pod unit.  Only one of its
// members may be specified.  If none of them is specified, the
// default one is UnitStateRunning.
type UnitState struct {
	Waiting    *UnitStateWaiting
	Running    *UnitStateRunning    `json:"running,omitempty"`
	Terminated *UnitStateTerminated `json:"terminated,omitempty"`
}

type UnitStatus struct {
	Name         string    `json:"name"`
	State        UnitState `json:"state,omitempty"`
	RestartCount int32     `json:"restartCount"`
	Image        string    `json:"image"`
}
