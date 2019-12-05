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
	cloudInfo, err = NewAzureCloudInfo("")
	if err == nil {
		return cloudInfo, nil
	}
	// We only support AWS and Azure.
	return nil, fmt.Errorf("unable to identify cloud platform")
}
