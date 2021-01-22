/*
Copyright 2020 Elotl Inc

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

package metrics

import (
	"github.com/elotl/itzo/pkg/api"
)

type Metrics struct {
	prevBusy float32
	prevAll  float32
}

func New() *Metrics {
	return &Metrics{}
}

// The generic system metrics provider uses psutil to gather data from the
// host system.
type GenericSystemMetricsProvider struct {
	// used to calculate cpu utilization percentage
	prevBusy float32
	prevAll  float32
}

func (m *GenericSystemMetricsProvider) ReadSystemMetrics(netif string) api.ResourceMetrics {
	// TODO
	return nil
}

// GetSystemMetrics returns a ResourceMetrics map with various pod and system
// level metrics.
func (m *Metrics) GetSystemMetrics(netif string) api.ResourceMetrics {
	return api.ResourceMetrics{}
}

// GetUnitMetrics returns a ResourceMetrics map with various container level
// metrics.
func (m *Metrics) GetUnitMetrics(name string) api.ResourceMetrics {
	return api.ResourceMetrics{}
}

type AnkaMetricsProvider struct {
	GenericSystemMetricsProvider
}

func (a *AnkaMetricsProvider) ReadUnitMetrics(ifname string) api.ResourceMetrics {
	// TODO
	return api.ResourceMetrics{}
}