package metrics

import "github.com/elotl/itzo/pkg/api"

// A metrics provider gather system and unit information on the host system and
// return a mapping of the successfully processed metrics.
type MetricsProvider interface {
	ReadSystemMetrics(string) api.ResourceMetrics
	ReadUnitMetrics(string) api.ResourceMetrics
}

