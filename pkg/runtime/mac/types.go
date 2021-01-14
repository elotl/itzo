package mac

const (
	AnkaStatusFail  = "FAIL"
	AnkaStatusError = "ERROR"
	AnkaStatusOK    = "OK"
)

type StartVMBody struct {
	VMID         string            `json:"vmid"`
	Tag          string            `json:"tag,omitempty"`
	Count        int               `json:"count,omitempty"`
	Name         string            `json:"name,omitempty"`
	CPU          int               `json:"vpcu,omitempty"`
	RAM          int               `json:"vram,omitempty"`
	NodeID       string            `json:"node_id,omitempty"`
	NameTemplate string            `json:"name_template,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type DeleteVMBody struct {
	VMID string `json:"id"`
}

type StartVMResp struct {
	VMRespBase
	IDs []string `json:"body"`
}

type VMInfoBase struct {
	UUID string `json:"uuid"`
}

type VMInfo struct {
	VMInfoBase
	NodeID   string `json:"node_id"`
	CpuCores int    `json:"cpu_cores"`
	IP       string `json:"ip"`
	Status   string `json:"status"`
	// TODO add all fields
}

type VMStatusBody struct {
	Progress int    `json:"progress"`
	VMInfo   VMInfo `json:"vminfo"`
}

type VMStatusResp struct {
	VMRespBase
	Body VMStatusBody `json:"body,omitempty"`
}

type VMStatusEmptyResp struct {
	VMRespBase
	Body []string `json:"body"`
}

type VMRespBase struct {
	Message string `json:"message"`
	Status  string `json:"status"`
}

type VMCreateOutput struct {
	VMRespBase
	Body VMInfoBase `json:"body"`
}

type VMStartOutput struct {
	VMRespBase `json:",inline"`
	Body       VMShowBody `json:",inline"`
}

type VMStopOutput struct {
	VMRespBase `json:",inline"`
	// TODO - add rest of fields
}

type VMShowBody struct {
	VMRespBase
	Name         string `json:"name"`
	CreationDate string `json:"creation_date"`
	CPUCores     int    `json:"cpu_cores"`
	CPUFrequency int    `json:"cpu_frequency"`
	CPUHtt       bool   `json:"cpu_htt"`
	RAM          string `json:"ram"`
	RAMSize      int    `json:"ram_size"`
	FrameBuffers int    `json:"frame_buffers"`
	HardDrive    int    `json:"hard_drive"`
	ImageSize    int    `json:"image_size"`
	Encrypted    bool   `json:"encrypted"`
	Status       string `json:"status"`
	StopDate     string `json:"stop_date"`
}

type VMShowOutput struct {
	VMRespBase `json:",inline"`
	Body       VMShowBody `json:",inline"`
	// {"status": "OK", "body": {"uuid": "617c1ddf-5645-4947-90f0-e8f82dd1c9fb", "name": "test-vm", "creation_date": "2021-01-07T12:05:58Z", "cpu_cores": 1, "cpu_frequency": 0, "cpu_htt": false, "ram": "1G", "ram_size": 1073741824, "frame_buffers": 1, "hard_drive": 137438953472, "image_size": 11014144, "encrypted": false, "status": "stopped", "stop_date": "2021-01-07T12:05:58.040770Z"}, "message": ""}
}

type VMPullOutput struct {
	VMRespBase `json:",inline"`
}

type VMRunOutput struct {
	VMRespBase `json:",inline"`
}
