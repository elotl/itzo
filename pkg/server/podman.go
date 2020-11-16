package server

import (
	"context"
	"github.com/containers/libpod/v2/pkg/bindings/images"
	"github.com/containers/libpod/v2/pkg/domain/entities"
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

