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

package net

import "k8s.io/kubernetes/pkg/util/iptables"

const (
	podMasqChain = "POD_MASQ_CHAIN"
)

// Enable SNAT so the pod can communicate with the public internet. E.g.
// if the pod IP is 10.0.30.14:
//
//     iptables -t nat -N POD_MASQ_CHAIN
//     iptables -t nat -A POSTROUTING -j POD_MASQ_CHAIN
//     iptables -t nat -A POD_MASQ_CHAIN ! -o eth0 -j RETURN
//     iptables -t nat -A POD_MASQ_CHAIN ! -s 10.0.30.14 -j RETURN
//     iptables -t nat -A POD_MASQ_CHAIN -d 10.0.0.0/8 -j RETURN
//     iptables -t nat -A POD_MASQ_CHAIN -d 172.16.0.0/12 -j RETURN
//     iptables -t nat -A POD_MASQ_CHAIN -d 192.168.0.0/16 -j RETURN
//     iptables -t nat -A POD_MASQ_CHAIN -j MASQUERADE
func EnsurePodMasq(ipt iptables.Interface, mainNic, podIP string) error {
	if mainNic == "" {
		var err error
		mainNic, err = GetPrimaryNetworkInterface()
		if err != nil {
			return err
		}
	}
	if _, err := ipt.EnsureChain(iptables.TableNAT, podMasqChain); err != nil {
		return err
	}
	if err := ipt.FlushChain(iptables.TableNAT, podMasqChain); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, iptables.ChainPostrouting, "-j", string(podMasqChain)); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "!", "-o", mainNic, "-j", "RETURN"); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "!", "-s", podIP, "-j", "RETURN"); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "-d", "10.0.0.0/8", "-j", "RETURN"); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "-d", "172.16.0.0/12", "-j", "RETURN"); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "-d", "192.168.0.0/16", "-j", "RETURN"); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.Append, iptables.TableNAT, podMasqChain, "-j", "MASQUERADE"); err != nil {
		return err
	}
	return nil
}
