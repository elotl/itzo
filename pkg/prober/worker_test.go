package prober

import (
	"fmt"
	"testing"
	"time"

	"github.com/elotl/itzo/pkg/api"
	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/probe"
)

// create a new worker
// create a fake exec prober

func makeTestWorker(probeType ProbeType, probe *api.Probe, result probe.Result, expectedError error) *worker {
	w := NewWorker("testUnit", probeType, probe)
	w.prober.exec = fakeExecProber{result, expectedError}
	w.startedAt = time.Now()
	return w
}

func mkProbe(spec api.Probe) api.Probe {
	spec.Handler = api.Handler{
		Exec: &api.ExecAction{},
	}
	if spec.TimeoutSeconds == 0 {
		spec.TimeoutSeconds = 1
	}
	if spec.PeriodSeconds == 0 {
		spec.PeriodSeconds = 1
	}
	if spec.SuccessThreshold == 0 {
		spec.SuccessThreshold = 1
	}
	if spec.FailureThreshold == 0 {
		spec.FailureThreshold = 1
	}
	return spec
}

func TestDoProbe(t *testing.T) {
	tests := []struct {
		probe             api.Probe
		probeResult       probe.Result
		probeError        error
		expectedHasResult bool
		expectedResult    Result
	}{
		{
			probe:             mkProbe(api.Probe{}),
			probeResult:       probe.Success,
			expectedHasResult: true,
			expectedResult:    Success,
		},
	}
	for i, tc := range tests {
		w := makeTestWorker(Readiness, &tc.probe, tc.probeResult, tc.probeError)
		hasRes, res := w.doProbe()
		msg := fmt.Sprintf("test %d", i)
		assert.Equal(t, tc.expectedHasResult, hasRes, msg)
		assert.Equal(t, tc.expectedResult, res, msg)
	}
}

func TestInitialDelay(t *testing.T) {
	for _, probeType := range [...]ProbeType{Liveness, Readiness, Startup} {
		p := mkProbe(api.Probe{InitialDelaySeconds: 10})
		w := makeTestWorker(probeType, &p, probe.Success, nil)
		hasRes, res := w.doProbe()
		assert.True(t, hasRes)
		switch probeType {
		case Liveness:
			assert.Equal(t, Success, res)
		case Readiness:
			assert.Equal(t, Failure, res)
		case Startup:
			assert.Equal(t, Unknown, res)
		}

		// 100 seconds later...
		w.startedAt = time.Now().Add(-100 * time.Second)
		hasRes, res = w.doProbe()
		assert.True(t, hasRes)
		assert.Equal(t, Success, res)
	}
}

func TestFailureThreshold(t *testing.T) {
	p := mkProbe(api.Probe{SuccessThreshold: 1, FailureThreshold: 3})
	w := makeTestWorker(Readiness, &p, probe.Success, nil)
	for i := 0; i < 2; i++ {
		// First probe should succeed.
		w.prober.exec = fakeExecProber{probe.Success, nil}

		for j := 0; j < 3; j++ {
			msg := fmt.Sprintf("%d success (%d)", j+1, i)
			hasRes, res := w.doProbe()
			assert.True(t, hasRes, msg)
			assert.Equal(t, Success, res, msg)
		}

		// Prober starts failing :(
		w.prober.exec = fakeExecProber{probe.Failure, nil}

		// Next 2 probes should still be "success".
		for j := 0; j < 2; j++ {
			msg := fmt.Sprintf("%d failing (%d)", j+1, i)
			hasRes, _ := w.doProbe()
			assert.False(t, hasRes, msg)
		}

		// Third & following fail.
		for j := 0; j < 3; j++ {
			msg := fmt.Sprintf("%d failure (%d)", j+3, i)
			hasRes, res := w.doProbe()
			assert.True(t, hasRes, msg)
			assert.Equal(t, Failure, res, msg)
		}
	}
}

func TestSuccessThreshold(t *testing.T) {
	p := mkProbe(api.Probe{SuccessThreshold: 3, FailureThreshold: 1})
	w := makeTestWorker(Readiness, &p, probe.Success, nil)
	w.lastResult = Failure
	for i := 0; i < 2; i++ {
		// Probe defaults to Failure.
		for j := 0; j < 2; j++ {
			msg := fmt.Sprintf("%d success (%d)", j+1, i)
			hasRes, _ := w.doProbe()
			assert.False(t, hasRes, msg)
		}

		// Continuing success!
		for j := 0; j < 3; j++ {
			msg := fmt.Sprintf("%d success (%d)", j+3, i)
			hasRes, res := w.doProbe()
			assert.True(t, hasRes, msg)
			assert.Equal(t, Success, res, msg)
		}

		// Prober flakes :(
		w.prober.exec = fakeExecProber{probe.Failure, nil}
		msg := fmt.Sprintf("1 failure (%d)", i)
		hasRes, res := w.doProbe()
		assert.True(t, hasRes, msg)
		assert.Equal(t, Failure, res, msg)

		// Back to success.
		w.prober.exec = fakeExecProber{probe.Success, nil}
	}
}

func TestRunProbeWorker(t *testing.T) {
	p := mkProbe(api.Probe{})
	w := makeTestWorker(Readiness, &p, probe.Success, nil)
	w.Start()
	select {
	case result := <-w.Results():
		assert.Equal(t, Success, result)
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Timed out waiting for success")
	}
	w.Stop()
}
