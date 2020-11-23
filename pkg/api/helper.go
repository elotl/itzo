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

package api

const PodName string = "itzopod"

func IsHostNetwork(securityContext *PodSecurityContext) bool {
	if securityContext == nil {
		return false
	}
	if securityContext.NamespaceOptions == nil ||
		securityContext.NamespaceOptions.Network != NamespaceModeNode {
		return false
	}
	return true
}

func MakeStillCreatingStatus(name, image, reason string) *UnitStatus {
	return &UnitStatus{
		Name: name,
		State: UnitState{
			Waiting: &UnitStateWaiting{
				Reason: reason,
			},
		},
		RestartCount: 0,
		Image:        image,
	}
}

func MakeFailedUpdateStatus(name, image, reaseon string) *UnitStatus {
	return &UnitStatus{
		Name: name,
		State: UnitState{
			Waiting: &UnitStateWaiting{
				Reason:       reaseon,
				StartFailure: true,
			},
		},
		Image: image,
	}
}
