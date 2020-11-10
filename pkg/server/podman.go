package server

import (
	"context"
	"github.com/containers/libpod/v2/libpod/define"
	"github.com/containers/libpod/v2/pkg/bindings/images"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/elotl/itzo/pkg/api"
	"github.com/golang/glog"
)

type PodmanPuller struct {
	connText context.Context
}

func NewPodmanPuller(connText context.Context) *PodmanPuller {
	return &PodmanPuller{connText: connText}
}


func (pp *PodmanPuller) PullImage(rootdir, name, image, server, username, password string) error {
	pulledImages, err := images.Pull(pp.connText, image, entities.ImagePullOptions{})
	if err != nil {
		glog.Errorf("error pulling podman image: %v", err)
		return err
	}
	glog.Infof("podman pulled images: %s", pulledImages)
	return nil
}

func ContainerStateToUnit(ctrData define.InspectContainerData) api.UnitState {
	if ctrData.State != nil {
		state := *ctrData.State
		return api.UnitState{
				Waiting:    &api.UnitStateWaiting{
					Reason:       state.Status,
					StartFailure: false,
				},
				Running:    &api.UnitStateRunning{StartedAt: api.Time{Time: state.StartedAt}},
				Terminated: &api.UnitStateTerminated{
					ExitCode:   state.ExitCode,
					FinishedAt: api.Time{Time: state.FinishedAt},
					// TODO send better reason
					Reason:     state.Status,
					Message:    state.Error,
					StartedAt:  api.Time{Time: state.StartedAt},
				},
			}
	}
	return api.UnitState{}
}