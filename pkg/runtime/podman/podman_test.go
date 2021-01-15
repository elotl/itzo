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

package podman

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"testing"

	"github.com/containers/podman/v2/pkg/bindings/images"
	"github.com/stretchr/testify/assert"
)

var testPodman = flag.Bool("podman", false, "run podman tests")

func image_exists(conn context.Context, imageName string) bool {
	exists, err := images.Exists(conn, imageName)
	if err != nil {
		panic(err)
	}
	return exists
}

func TestPodmanPullImageNoCredentials(t *testing.T) {
	if !*testPodman {
		return
	}

	const imageName = "docker.io/library/hello-world"

	tmpdir, err := ioutil.TempDir("", "podman-test")
	if err != nil {
		t.Fatalf("Error creating temporary directory")
	}
	defer os.RemoveAll(tmpdir)
	conn, err := GetPodmanConnection()
	if err != nil {
		t.Fatalf("Can't get podman connection: %v", err)
	}
	var p = NewPodmanContainerService(conn, tmpdir)

	if image_exists(conn, imageName) {
		_, err = images.Remove(conn, imageName, false)
		if err != nil {
			panic(err)
		}
		if image_exists(conn, imageName) {
			t.Fatalf("Image %s hasn't been removed", imageName)
		}
	}

	assert.NoError(t, p.imgPuller.PullImage(tmpdir, "container", imageName, nil, false))

	if !image_exists(conn, imageName) {
		t.Fatalf("PullImage successful, but image doesn't exist")
	}
}
