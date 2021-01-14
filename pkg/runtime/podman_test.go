package runtime

import (
	"context"
	"flag"
	"github.com/containers/podman/v2/pkg/bindings/images"
	"io/ioutil"
	"os"
	"testing"
)

var testPodman = flag.Bool("podman", false, "run podman tests")

func image_exists(conn context.Context, imageName string) bool {
	exists, err := images.Exists(conn, imageName)
	if err != nil {
		panic(err)
	}
	return exists
}

// This test takes a long time to execute. Around 14 second on my computer.
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
	p.imgPuller.PullImage(tmpdir, "container", imageName, nil, false)

	if !image_exists(conn, imageName) {
		t.Fatalf("PullImage successful, but image doesn't exist")
	}
}
