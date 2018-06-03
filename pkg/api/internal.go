package api

type PodParameters struct {
	Secrets     map[string]map[string][]byte   `json:"secrets"`
	Credentials map[string]RegistryCredentials `json:"credentials"`
	Spec        PodSpec                        `json:"spec"`
}

type RegistryCredentials struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type PodStatusReply struct {
	UnitStatuses []UnitStatus `json:"unitStatus"`
}

type PortForwardParams struct {
	PodName string
	Port    string
}

type ExecParams struct {
	PodName     string
	UnitName    string
	Command     string
	Interactive bool
	TTY         bool
}

type RunCmdParams struct {
	Command []string
}
