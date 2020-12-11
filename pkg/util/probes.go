package util

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Our probes can reference unit ports by the name given to them in
// the unit.  However, where the probes are actually used (inside
// unit.go) we don't have access to the full unit structure. Here we
// make a copy of an httpget probe and update it to use only port
// numbers. If we can't look up a probe's name, we don't fail the
// unit, instead we pass along the unmodified port and it'll fail in
// the probe. That matches the behavior of the kubelet.  Note that we
// can't change a probe in-place since that messes up the diffs we do
// on pods during UpdatePod and would cause us to delete and recreate
// pods.
func TranslateProbePorts(unit *api.Unit, probe *api.Probe) *api.Probe {
	// only translate the port name if this is an http probe with a port
	// with a string name
	if probe != nil &&
		probe.HTTPGet != nil &&
		probe.HTTPGet.Port.Type == intstr.String {
		p := *probe
		port, err := FindPortByName(unit, p.HTTPGet.Port.StrVal)
		if err != nil {
			glog.Errorf("error looking up probe port: %s", err)
		} else {
			newAction := *p.HTTPGet
			p.HTTPGet = &newAction
			p.HTTPGet.Port = intstr.FromInt(port)
		}
		return &p
	} else {
		return probe
	}
}

