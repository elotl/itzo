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
	"strings"
)

// Parse the image spec and extract the registry server and the repo name.
// Example image specs:
//   ECS: ACCOUNT.dkr.ecr.REGION.amazonaws.com/imagename:tag
//   Docker Hub: imagename:tag or owner/imagename:tag
func ParseImageSpec(image string) (string, string, error) {
	server := ""
	repo := image
	var err error
	parts := strings.Split(image, "/")
	if len(parts) == 1 {
		repo = strings.Join([]string{"library", parts[0]}, "/")
	} else if strings.Contains(parts[0], ".") {
		server = parts[0]
		repo = strings.Join(parts[1:], "/")
	}
	return server, repo, err
}
