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

package util

import (
	"github.com/elotl/itzo/pkg/api"
	"github.com/golang/glog"
	"strings"
)

// Parse the image spec and extract the registry server and the repo name.
// Example image specs:
//   ECS: ACCOUNT.dkr.ecr.REGION.amazonaws.com/imagename:tag
//   Docker Hub: imagename:tag or owner/imagename:tag
func ParseImageSpec(image string) (string, string) {
	server := ""
	repo := image
	parts := strings.Split(image, "/")
	if len(parts) == 1 {
		repo = strings.Join([]string{"library", parts[0]}, "/")
	} else if strings.Contains(parts[0], ".") {
		server = parts[0]
		repo = strings.Join(parts[1:], "/")
	}
	glog.Infof("image: %s parsed to server: %s repo: %s", image, server, repo)
	return server, repo
}

// Dockerhub can go by several names that the user can specify in
// their image spec.  .docker/config.json uses index.docker.io but an
// empty server should also map to that. We have used
// registry-1.docker.io internally and users might also just say
// "docker.io".  Try to find the server that shows up in the
// credentials sent over from kip.
func GetRepoCreds(server string, allCreds map[string]api.RegistryCredentials) (string, string) {
	if creds, ok := allCreds[server]; ok {
		return creds.Username, creds.Password
	}
	// if credentials weren't found and the server is dockerhub
	// (server is empty or ends in docker.io), try to find credentials
	// for (in this order): index.docker.io, registry-1.docker.io,
	// docker.io
	if server == "" || strings.HasSuffix(server, "docker.io") {
		possibleServers := []string{"index.docker.io", "registry-1.docker.io", "docker.io"}
		for _, possibleServer := range possibleServers {
			if creds, ok := allCreds[possibleServer]; ok {
				return creds.Username, creds.Password
			}
		}
	}
	return "", ""
}

