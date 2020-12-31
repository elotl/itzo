package mac

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

const (
	ankaCLIPath = "/usr/bin/anka"
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

func (ac *AnkaCLI) Create(vmTemplateID string) (VMCreateOutput, error) {
	// Anka Build licenses default your VM in a suspended state
	// anka --machine-readable create --ram-size=1G --cpu-count=1 test-vm
	cmd := ac.buildCmd("create", []string{vmTemplateID})
	var output VMCreateOutput
	bytesOutput, err := ac.run(cmd)
	if err != nil {
		return VMCreateOutput{}, err
	}
	err = json.Unmarshal(bytesOutput, &output)
	if err != nil {
		return VMCreateOutput{}, err
	}
	if output.Status != AnkaStatusOK {
		return VMCreateOutput{}, fmt.Errorf("VM creation failed with: %s", output.Message)
	}
	return output, err
}

func (ac *AnkaCLI) Show(vmID string) (VMShowOutput, error) {
	// anka --machine-readable show 617c1ddf-5645-4947-90f0-e8f82dd1c9fb
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
		outputFlag,
		ankaCommand,
	}
	args = append(args, extraArgs...)
	cmd := exec.Command(ankaCLIPath, args...)
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
	cmd := exec.Command(ankaCLIPath, "version")
	err := cmd.Run()
	return err
}
