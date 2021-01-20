package mac

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"os/exec"
	"testing"
)

func TestParseImageUrl(t *testing.T) {
	testCases := []struct {
		name        string
		inputUrl    string
		expectedUrl string
	}{
		{
			name:        "no port no proto",
			inputUrl:    "registry.default.svc/vm-id",
			expectedUrl: "http://registry.default.svc/registry/v2/vm?id=vm-id",
		},
		{
			name:        "port no proto",
			inputUrl:    "registry.default.svc:8089/vm-id",
			expectedUrl: "http://registry.default.svc:8089/registry/v2/vm?id=vm-id",
		},
		{
			name:        "port proto",
			inputUrl:    "http://registry.default.svc:8089/vm-id",
			expectedUrl: "http://registry.default.svc:8089/registry/v2/vm?id=vm-id",
		},
		{
			name:        "no port proto",
			inputUrl:    "http://registry.default.svc/vm-id",
			expectedUrl: "http://registry.default.svc/registry/v2/vm?id=vm-id",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotVMId, gotTemplateUrl := parseImageUrl(testCase.inputUrl)
			assert.Equal(t, testCase.expectedUrl, gotTemplateUrl)
			assert.Equal(t, "vm-id", gotVMId)
		})
	}
}

func TestAnkaCLI_ActivateLicense(t *testing.T) {
	testCases := []struct{
		name string
		cmdOutput string
		shouldErr bool
	}{
		{
			name: "happy path",
			cmdOutput: "{\"status\": \"OK\", \"body\": {}, \"message\": \"License activated\"}",
			shouldErr: false,
		},
		{
			name: "error",
			cmdOutput: "{\"status\": \"ERROR\", \"body\": {}, \"message\": \"Activation failed\"}",
			shouldErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cliClient := NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
				output.Write([]byte(testCase.cmdOutput))
				return nil
			})
			err := cliClient.ActivateLicense("dummy")
			if testCase.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAnkaCLI_ValidateLicense(t *testing.T) {
	testCases := []struct{
		name string
		cmdOutput string
		shouldErr bool
	}{
		{
			name: "happy path",
			cmdOutput: "{\"status\": \"OK\", \"body\": {}, \"message\": \"License is valid!\"}",
			shouldErr: false,
		},
		{
			name: "error",
			cmdOutput: "{\"status\": \"ERROR\", \"body\": {}, \"message\": \"License failed\"}",
			shouldErr: true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cliClient := NewAnkaCLI(func(cmd *exec.Cmd, output *bytes.Buffer) error {
				output.Write([]byte(testCase.cmdOutput))
				return nil
			})
			err := cliClient.ValidateLicense()
			if testCase.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}