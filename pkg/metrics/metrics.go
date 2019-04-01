package metrics

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

type Metrics struct {
	prevBusy float32
	prevAll  float32
}

func New() *Metrics {
	return &Metrics{}
}

// There are a couple of measurements of CPU we could use
// 1. cpuPercent(): Avg CPU% across all CPUs (cpuPercent())
// 2. Max CPU% across all CPUs
// 3. cpuUtilization(): percent of the machine's reported power that
//    is actually used. Not reported correctly on Azure

func (m *Metrics) cpuUtilization() float32 {
	cpuTimes, err := cpu.Times(false)
	if err != nil || len(cpuTimes) == 0 {
		return 0.0
	}
	c := cpuTimes[0]
	curBusy := float32(c.Total() - c.Steal - c.Idle)
	curAll := float32(c.Total())
	if curAll <= m.prevAll || curBusy < m.prevBusy {
		return float32(0.0)
	}
	utilizationPercent := (curBusy - m.prevBusy) / (curAll - m.prevAll) * 100
	m.prevBusy = curBusy
	m.prevAll = curAll
	return utilizationPercent
}

func (m *Metrics) cpuPercent() float32 {
	percents, err := cpu.Percent(0, true)
	if err != nil || len(percents) == 0 {
		return 0.0
	}
	return float32(percents[0])
}

// We choose percents over quantities since those values remain
// constant for an autoscaler when you change your instance type.
// Thus, if you vertically scale your application to double the CPU
// count, but keep the autoscaling values the same, you should still
// scale at the right place.
//
// At this time, We don't use cpuUtilization (including steal) like
// AWS reports in cloudWatch since steal values dont come through in
// Azure (hyper-v).
func (m *Metrics) GetSystemMetrics() api.PodMetrics {
	metrics := api.PodMetrics{}
	if memoryStats, err := mem.VirtualMemory(); err == nil {
		metrics.Memory = float32(memoryStats.UsedPercent)
	}
	if diskStats, err := disk.Usage("/"); err == nil {
		metrics.Disk = float32(diskStats.UsedPercent)
	}
	metrics.CPU = m.cpuPercent()
	return metrics
}
