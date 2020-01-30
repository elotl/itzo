package prober

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"k8s.io/kubernetes/pkg/probe"
	execprobe "k8s.io/kubernetes/pkg/probe/exec"
	kubeexec "k8s.io/utils/exec"
)

func TestFormatURL(t *testing.T) {
	testCases := []struct {
		scheme string
		host   string
		port   int
		path   string
		result string
	}{
		{"http", "localhost", 93, "", "http://localhost:93"},
		{"https", "localhost", 93, "/path", "https://localhost:93/path"},
		{"http", "localhost", 93, "?foo", "http://localhost:93?foo"},
		{"https", "localhost", 93, "/path?bar", "https://localhost:93/path?bar"},
	}
	for _, test := range testCases {
		url := formatURL(test.scheme, test.host, test.port, test.path)
		if url.String() != test.result {
			t.Errorf("Expected %s, got %s", test.result, url.String())
		}
	}
}

func TestHTTPHeaders(t *testing.T) {
	testCases := []struct {
		input  []api.HTTPHeader
		output http.Header
	}{
		{[]api.HTTPHeader{}, http.Header{}},
		{[]api.HTTPHeader{
			{Name: "X-Muffins-Or-Cupcakes", Value: "Muffins"},
		}, http.Header{"X-Muffins-Or-Cupcakes": {"Muffins"}}},
		{[]api.HTTPHeader{
			{Name: "X-Muffins-Or-Cupcakes", Value: "Muffins"},
			{Name: "X-Muffins-Or-Plumcakes", Value: "Muffins!"},
		}, http.Header{"X-Muffins-Or-Cupcakes": {"Muffins"},
			"X-Muffins-Or-Plumcakes": {"Muffins!"}}},
		{[]api.HTTPHeader{
			{Name: "X-Muffins-Or-Cupcakes", Value: "Muffins"},
			{Name: "X-Muffins-Or-Cupcakes", Value: "Cupcakes, too"},
		}, http.Header{"X-Muffins-Or-Cupcakes": {"Muffins", "Cupcakes, too"}}},
	}
	for _, test := range testCases {
		headers := buildHeader(test.input)
		if !reflect.DeepEqual(test.output, headers) {
			t.Errorf("Expected %#v, got %#v", test.output, headers)
		}
	}
}

type fakeExecRunner struct {
	cmd []string
}

func (r *fakeExecRunner) RunWithTimeout(cmd []string, timeout time.Duration) ([]byte, error) {
	r.cmd = cmd
	return nil, nil
}

type fakeExecProber struct {
	result probe.Result
	err    error
}

func (p fakeExecProber) Probe(c kubeexec.Cmd) (probe.Result, string, error) {
	return p.result, "", p.err
}

func TestProbe(t *testing.T) {
	execProbe := &api.Probe{
		Handler: api.Handler{
			Exec: &api.ExecAction{},
		},
	}
	tests := []struct {
		probe          *api.Probe
		env            []api.EnvVar
		execError      bool
		expectError    bool
		execResult     probe.Result
		expectedResult Result
		expectCommand  []string
	}{
		{ // No probe
			probe:          nil,
			expectedResult: Success,
		},
		{ // No handler
			probe:          &api.Probe{},
			expectError:    true,
			expectedResult: Failure,
		},
		{ // Probe fails
			probe:          execProbe,
			execResult:     probe.Failure,
			expectedResult: Failure,
		},
		{ // Probe succeeds
			probe:          execProbe,
			execResult:     probe.Success,
			expectedResult: Success,
		},
		{ // Probe result is warning
			probe:          execProbe,
			execResult:     probe.Warning,
			expectedResult: Success,
		},
		{ // Probe result is unknown
			probe:          execProbe,
			execResult:     probe.Unknown,
			expectedResult: Failure,
		},
		{ // Probe has an error
			probe:          execProbe,
			execError:      true,
			expectError:    true,
			execResult:     probe.Unknown,
			expectedResult: Failure,
		},
		{ // Probe arguments are passed through
			probe: &api.Probe{
				Handler: api.Handler{
					Exec: &api.ExecAction{
						Command: []string{"/bin/bash", "-c", "some script"},
					},
				},
			},
			expectCommand:  []string{"/bin/bash", "-c", "some script"},
			execResult:     probe.Success,
			expectedResult: Success,
		},
		{ // Probe arguments are passed through
			probe: &api.Probe{
				Handler: api.Handler{
					Exec: &api.ExecAction{
						Command: []string{"/bin/bash", "-c", "some $(A) $(B)"},
					},
				},
			},
			env: []api.EnvVar{
				{Name: "A", Value: "script"},
			},
			expectCommand:  []string{"/bin/bash", "-c", "some script $(B)"},
			execResult:     probe.Success,
			expectedResult: Success,
		},
	}

	for i, test := range tests {
		for _, probeType := range [...]ProbeType{Liveness, Readiness, Startup} {
			prober := &prober{
				unitEnv: test.env,
			}
			testID := fmt.Sprintf("%d-%s", i, probeType)
			if test.execError {
				prober.exec = fakeExecProber{test.execResult, errors.New("exec error")}
			} else {
				prober.exec = fakeExecProber{test.execResult, nil}
			}

			result, err := prober.probe(probeType, test.probe)
			if test.expectError && err == nil {
				t.Errorf("[%s] Expected probe error but no error was returned.", testID)
			}
			if !test.expectError && err != nil {
				t.Errorf("[%s] Didn't expect probe error but got: %v", testID, err)
			}
			if test.expectedResult != result {
				t.Errorf("[%s] Expected result to be %v but was %v", testID, test.expectedResult, result)
			}

			if len(test.expectCommand) > 0 {
				prober.exec = execprobe.New()
				prober.runner = &fakeExecRunner{}
				_, err := prober.probe(probeType, test.probe)
				if err != nil {
					t.Errorf("[%s] Didn't expect probe error but got: %v", testID, err)
					continue
				}
				sentCmd := prober.runner.(*fakeExecRunner).cmd
				if !reflect.DeepEqual(test.expectCommand, sentCmd) {
					t.Errorf("[%s] unexpected probe arguments: %v", testID, sentCmd)
				}
			}
		}
	}
}
