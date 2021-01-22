package mac

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

const (
	ankaCLIPath = "/usr/local/bin/anka"
	outputFlag  = "--machine-readable"
)

type CommandExecutor func(cmd *exec.Cmd, output *bytes.Buffer) error

type AnkaCLI struct {
	executor CommandExecutor
}

func NewAnkaCLI(executor CommandExecutor) *AnkaCLI {
	return &AnkaCLI{executor: executor}
}

func CMDExecWrapper(cmd *exec.Cmd, output *bytes.Buffer) error {
	cmd.Stdout = output
	err := cmd.Run()
	return err
}

//func (ac *AnkaCLI) Create(vmTemplateID string) (VMCreateOutput, error) {
//	// Anka Build licenses default your VM in a suspended state
//	// anka --machine-readable create --ram-size=1G --cpu-count=1 test-vm
//	cmd := ac.buildCmd("create", []string{vmTemplateID})
//	var output VMCreateOutput
//	bytesOutput, err := ac.run(cmd)
//	if err != nil {
//		return VMCreateOutput{}, err
//	}
//	err = json.Unmarshal(bytesOutput, &output)
//	if err != nil {
//		return VMCreateOutput{}, err
//	}
//	if output.Status != AnkaStatusOK {
//		return VMCreateOutput{}, fmt.Errorf("VM creation failed with: %s", output.Message)
//	}
//	return output, err
//}

func (ac *AnkaCLI) Show(vmID string) (VMShowOutput, error) {
	// anka --machine-readable show 617c1ddf-5645-4947-90f0-e8f82dd1c9fb
	// {"status": "OK", "body": {"uuid": "346ead67-f3cc-4538-848f-0e90b255c854", "name": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032", "creation_date": "2021-01-15T15:26:59Z", "cpu_cores": 3, "cpu_frequency": 0, "cpu_htt": false, "ram": "8G", "ram_size": 8589934592, "frame_buffers": 1, "hard_drive": 137438953472, "image_size": 528384, "encrypted": false, "status": "failed", "stop_date": "2021-01-15T15:43:00.594230Z"}, "message": ""}
	// {"status": "OK", "body": {"uuid": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032", "name": "10.15.7", "version": "base:port-forward-22:brew-git", "creation_date": "2020-12-23T03:40:07.548902Z", "cpu_cores": 3, "cpu_frequency": 0, "cpu_htt": false, "ram": "8G", "ram_size": 8589934592, "frame_buffers": 1, "hard_drive": 107374182400, "image_size": 18381783040, "encrypted": false, "addons_version": "2.3.1.124", "status": "running", "port_forwarding": [{"guest_port": 22, "host_port": 10000, "protocol": "tcp", "name": "ssh", "host_ip": "0.0.0.0"}], "mac": "2:79:13:1f:17:24", "vnc_port": 5900, "vnc_password": "admin", "vnc_connection_string": "vnc://172.31.28.178:5900", "pid": 3836, "start_date": "2021-01-15T15:48:42.110781Z"}, "message": ""}
	// {"status": "OK", "body": {"uuid": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032", "name": "10.15.7", "version": "base:port-forward-22:brew-git", "creation_date": "2020-12-23T03:40:07.548902Z", "cpu_cores": 3, "cpu_frequency": 0, "cpu_htt": false, "ram": "8G", "ram_size": 8589934592, "frame_buffers": 1, "hard_drive": 107374182400, "image_size": 18988908544, "encrypted": false, "addons_version": "2.3.1.124", "status": "running", "port_forwarding": [{"guest_port": 22, "host_port": 10000, "protocol": "tcp", "name": "ssh", "host_ip": "0.0.0.0"}], "ip": "192.168.128.2", "mac": "ce:fc:3c:f4:e8:db", "vnc_port": 5900, "vnc_password": "admin", "vnc_connection_string": "vnc://172.31.18.70:5900", "pid": 13221, "start_date": "2021-01-18T08:38:53.067971Z"}, "message": ""}
	cmd := ac.buildCmd("show", []string{vmID})
	var output VMShowOutput
	bytesOutput, err := ac.run(cmd)
	if err != nil {
		return VMShowOutput{}, err
	}
	err = json.Unmarshal(bytesOutput, &output)
	if err != nil {
		return VMShowOutput{}, err
	}
	return output, nil
}

func (ac *AnkaCLI) Start(vmId string) (VMStartOutput, error) {
	// anka --machine-readable start <vm-id>
    // {"status": "OK", "body": {"uuid": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032", "name": "10.15.7", "version": "base:port-forward-22:brew-git", "creation_date": "2020-12-23T03:40:07.548902Z", "cpu_cores": 3, "cpu_frequency": 0, "cpu_htt": false, "ram": "8G", "ram_size": 8589934592, "frame_buffers": 1, "hard_drive": 107374182400, "image_size": 18381783040, "encrypted": false, "addons_version": "2.3.1.124", "status": "running", "port_forwarding": [{"guest_port": 22, "host_port": 10000, "protocol": "tcp", "name": "ssh", "host_ip": "0.0.0.0"}], "mac": "2:79:13:1f:17:24", "vnc_port": 5900, "vnc_password": "admin", "vnc_connection_string": "vnc://172.31.18.70:5900", "pid": 4792, "start_date": "2021-01-18T08:17:45.002390Z"}, "message": ""}
	//{"status": "OK", "body": {"uuid": "346ead67-f3cc-4538-848f-0e90b255c854", "name": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032", "creation_date": "2021-01-15T15:26:59Z", "cpu_cores": 3, "cpu_frequency": 0, "cpu_htt": false, "ram": "8G", "ram_size": 8589934592, "frame_buffers": 1, "hard_drive": 137438953472, "image_size": 528384, "encrypted": false, "status": "running", "mac": "f6:20:5:e1:11:a1", "vnc_port": 5900, "vnc_password": "admin", "vnc_connection_string": "vnc://172.31.28.178:5900", "pid": 1362, "start_date": "2021-01-15T15:42:59.956857Z"}, "message": ""}
	cmd := ac.buildCmd("start", []string{vmId})
	var output VMStartOutput
	bytesOutput, err := ac.run(cmd)
	if err != nil {
		return VMStartOutput{}, err
	}
	err = json.Unmarshal(bytesOutput, &output)
	if err != nil {
		return VMStartOutput{}, err
	}
	return output, nil
}

func (ac *AnkaCLI) Stop(vmId string) (VMStopOutput, error) {
	cmd := ac.buildCmd("stop", []string{vmId})
	var output VMStopOutput
	bytesOutput, err := ac.run(cmd)
	if err != nil {
		return VMStopOutput{}, err
	}
	err = json.Unmarshal(bytesOutput, &output)
	if err != nil {
		return VMStopOutput{}, err
	}
	return output, nil
}

func (ac *AnkaCLI) PullImage(vmTemplateID string) error {
	// anka registry pull -l <vm-id>
	// {"status": "OK", "body": {"uuid": "c0847bc9-5d2d-4dbc-ba6a-240f7ff08032"}, "message": "vm pulled successfully"}
	cmd := ac.buildCmd("registry", []string{"pull", "-l", vmTemplateID})
	pullOutput, err := ac.run(cmd)
	if err != nil {
		return err
	}
	var output VMPullOutput
	err = json.Unmarshal(pullOutput, &output)
	if err != nil {
		return err
	}
	if output.Status != AnkaStatusOK {
		return fmt.Errorf("pulling failed with %s", output.Message)
	}
	return nil
}

func (ac *AnkaCLI) Exec(vmID string, command []string) error {
	extraArgs := []string{vmID}
	extraArgs = append(extraArgs, command...)
	cmd := ac.buildCmd("run", extraArgs)
	runOutput, err := ac.run(cmd)
	if err != nil {
		return err
	}
	var output VMRunOutput
	err = json.Unmarshal(runOutput, &output)
	if err != nil {
		return err
	}
	if output.Status != AnkaStatusOK {
		return fmt.Errorf("error running cmd in vm: %s", output.Message)
	}
	return nil
}

func (ac *AnkaCLI) ActivateLicense(licenseKey string) error {
	// {"status": "OK", "body": {}, "message": "License activated"}
	cmd := ac.buildCmd("license", []string{"activate", "-f", licenseKey})
	activateOutput, err := ac.run(cmd)
	if err != nil {
		return err
	}
	var output VMRespBase
	err = json.Unmarshal(activateOutput, &output)
	if err != nil {
		return err
	}
	if output.Status != AnkaStatusOK {
		return fmt.Errorf("activating failed: %s", output.Message)
	}
	if output.Message == "License activated" {
		return nil
	}
	return fmt.Errorf("activating failed: %s", output.Message)
}

func (ac *AnkaCLI) ValidateLicense() error {
	// {"status": "OK", "body": {}, "message": "License is valid!"}
	cmd := ac.buildCmd("license", []string{"validate"})
	validateOutput, err := ac.run(cmd)
	if err != nil {
		return err
	}
	var output VMRespBase
	err = json.Unmarshal(validateOutput, &output)
	if err != nil {
		return err
	}
	if output.Message == "License is valid!" && output.Status == AnkaStatusOK {
		return nil
	}
	return fmt.Errorf("license not valid: %s", output.Message)
}

func (ac *AnkaCLI) run(cmd *exec.Cmd) ([]byte, error) {
	var out bytes.Buffer
	err := ac.executor(cmd, &out)
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (ac *AnkaCLI) buildCmd(ankaCommand string, extraArgs []string) *exec.Cmd {
	args := []string{
		ankaCLIPath,
		outputFlag,
		ankaCommand,
	}
	args = append(args, extraArgs...)
	// anka throws an error if we run without sudo
	cmd := exec.Command("sudo", args...)
	return cmd
}

func (ac *AnkaCLI) parseOutput(data []byte, outStruct interface{}) (interface{}, error) {
	err := json.Unmarshal(data, &outStruct)
	if err != nil {
		return nil, err
	}
	return outStruct, nil
}

func (ac *AnkaCLI) EnsureAnkaBin() error {
	cmd := exec.Command("sudo", ankaCLIPath, "version")
	err := cmd.Run()
	return err
}
