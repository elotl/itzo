package api

type PodParameters struct {
	Secrets     map[string]map[string][]byte   `json:"secrets"`
	Credentials map[string]RegistryCredentials `json:"credentials"`
	Spec        PodSpec                        `json:"spec"`
	PodName     string
	ClusterName string
	Namespace   string
}

type RegistryCredentials struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type PodStatusReply struct {
	UnitStatuses     []UnitStatus `json:"unitStatus"`
	InitUnitStatuses []UnitStatus `json:"initUnitStatus"`
	Metrics          PodMetrics   `json:"metrics"`
}

type PodMetrics struct {
	CPU    float32 `json:"cpu"`
	Memory float32 `json:"memory"`
	Disk   float32 `json:"disk"`
}

type PortForwardParams struct {
	PodName string
	Port    string
}

type ExecParams struct {
	PodName     string
	UnitName    string
	Command     []string
	Interactive bool
	TTY         bool
	SkipNSEnter bool
}

type AttachParams struct {
	PodName     string
	UnitName    string
	Interactive bool
	TTY         bool
}

type RunCmdParams struct {
	Command []string
}
