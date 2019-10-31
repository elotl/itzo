package cloud

import "fmt"

type CloudInfo interface {
	GetPodIPv4Address() (string, error)
	GetMainIPv4Address() (string, error)
}

func NewCloudInfo() (CloudInfo, error) {
	cloudInfo, err := NewAwsCloudInfo()
	if err == nil {
		return cloudInfo, nil
	}
	// TODO: create metadata client for Azure.
	return nil, fmt.Errorf("unable to identify cloud platform")
}
