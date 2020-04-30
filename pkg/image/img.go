package image

import (
	"net/url"

	"github.com/elotl/itzo/pkg/util"
)

// This image client is non-functional right now, since it can't export image
// configs. It's only here for reference.

const (
	ImgMaxRetries     = 3
	ImgOutputLimit    = 4096
	ImgExe            = "img"
	ImgMinimumVersion = "v0.5.7"
	ImgURL            = "https://github.com/genuinetools/img/releases/download/v0.5.7/img-linux-amd64"
)

type Img struct {
	stateDir string
	exe      string
}

func NewImg(stateDir, imgExe string) *Img {
	return &Img{
		stateDir: stateDir,
		exe:      ImgExe,
	}
}

func (i *Img) Login(server, username, password string) error {
	return i.run(
		"login", "--username", username, "--password", password, server)
}

func (i *Img) Pull(server, image string) error {
	if server != "" {
		host := server
		u, err := url.Parse(server)
		if err == nil {
			host = u.Host
		}
		image = host + "/" + image
	}
	return i.run(
		"pull", "--state", i.stateDir, image)
}

func (i *Img) Unpack(image, dest, configPath string) error {
	return i.run(
		"unpack", "--state", i.stateDir, "--output", dest, image)
}

func (i *Img) run(args ...string) error {
	exe, err := util.EnsureProg(i.exe, ImgURL, ImgMinimumVersion, "version")
	if err != nil {
		return err
	}
	if exe != i.exe {
		i.exe = exe
	}
	return util.RunProg(exe, ImgOutputLimit, ImgMaxRetries, args...)
}
