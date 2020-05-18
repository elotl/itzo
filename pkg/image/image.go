package image

type ImageClient interface {
	Login(server, username, password string) error
	Pull(server, image string) error
	Unpack(image, dest string) error
}
