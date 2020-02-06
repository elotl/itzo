/*
Copyright 2015 The Kubernetes Authors.
Copyright 2020 Elotl Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package prober

import (
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/elotl/itzo/pkg/api"
)

type ProbeType int

const (
	Liveness ProbeType = iota
	Readiness
	Startup
)

// For debugging.
func (t ProbeType) String() string {
	switch t {
	case Readiness:
		return "Readiness"
	case Liveness:
		return "Liveness"
	case Startup:
		return "Startup"
	default:
		return "UNKNOWN"
	}
}

// worker handles the periodic probing of its assigned container. Each worker has a go-routine
// associated with it which runs the probe loop until the container permanently terminates, or the
// stop channel is closed. The worker uses the probe Manager's statusManager to get up-to-date
// container IDs.
type worker struct {
	// Channel for stopping the probe.
	stopCh chan struct{}

	// Describes the probe configuration (read-only)
	spec *api.Probe

	// The type of the worker.
	probeType ProbeType

	// The probe value during the initial delay.
	initialValue Result

	// What time the probe (and therefore unit) started at
	startedAt time.Time

	// The last probe result for this worker.
	lastResult Result

	// How many times in a row the probe has returned the same result.
	resultRun int

	prober *prober

	results chan Result
}

func EnvironToAPIEnvVar(envs []string) []api.EnvVar {
	apiEnv := make([]api.EnvVar, 0, len(envs))
	for i := range envs {
		items := strings.SplitN(envs[i], "=", 2)
		apiEnv = append(apiEnv, api.EnvVar{Name: items[0], Value: items[1]})
	}
	return apiEnv
}

// Creates and starts a new probe worker.
func NewWorker(
	unitName string,
	probeType ProbeType,
	probe *api.Probe) *worker {

	// apiEnv := util.EnvironToAPIEnvVar(os.Environ())
	apiEnv := EnvironToAPIEnvVar(os.Environ())
	prober := newProber(unitName, apiEnv)
	w := &worker{
		stopCh:    make(chan struct{}, 1), // make stop() non-blocking
		probeType: probeType,
		spec:      probe,
		results:   make(chan Result, 2), // also needs to be non-blocking
		prober:    prober,
	}

	switch probeType {
	case Readiness:
		w.initialValue = Failure
	case Liveness:
		w.initialValue = Success
	case Startup:
		w.initialValue = Unknown
	}
	return w
}

// run periodically probes the container.
func (w *worker) Start() {
	if w.spec == nil {
		return
	}
	go w.run()
}

func (w *worker) run() {
	w.startedAt = time.Now()
	probeTickerPeriod := time.Duration(w.spec.PeriodSeconds) * time.Second

	// If kubelet restarted the probes could be started in rapid succession.
	// Let the worker wait for a random portion of tickerPeriod before probing.
	time.Sleep(time.Duration(rand.Float64() * float64(probeTickerPeriod)))

	probeTicker := time.NewTicker(probeTickerPeriod)
	defer probeTicker.Stop()

probeLoop:
	for {
		hasResult, result := w.doProbe()
		if hasResult {
			w.emit(result)
		}
		// Wait for next probe tick.
		select {
		case <-w.stopCh:
			break probeLoop
		case <-probeTicker.C:
			// continue
		}
	}
}

// stop stops the probe worker. The worker handles cleanup and removes itself from its manager.
// It is safe to call stop multiple times.
func (w *worker) Stop() {
	select {
	case w.stopCh <- struct{}{}:
	default: // Non-blocking.
	}
}

// doProbe probes the container once and records the result.
// Returns whether the worker should continue.
func (w *worker) doProbe() (hasResult bool, result Result) {
	defer func() { recover() }()
	if int32(time.Since(w.startedAt).Seconds()) < w.spec.InitialDelaySeconds {
		return true, w.initialValue
	}

	result, err := w.prober.probe(w.probeType, w.spec)
	if err != nil {
		// Prober error, throw away the result.
		return false, result
	}

	if w.lastResult == result {
		w.resultRun++
	} else {
		w.lastResult = result
		w.resultRun = 1
	}

	if (result == Failure && w.resultRun < int(w.spec.FailureThreshold)) ||
		(result == Success && w.resultRun < int(w.spec.SuccessThreshold)) {
		// Success or failure is below threshold - leave the probe state unchanged
		return false, result
	}
	return true, result
}

func (w *worker) emit(result Result) {
	// Todo: unsure if we want a non-blocking send or if we should
	// buffer the channel. I've decided to buffer the channel as
	// that'll work out better in terms of races.
	select {
	case w.results <- result:
	default:
	}
}

func (w *worker) Results() chan Result {
	return w.results
}
