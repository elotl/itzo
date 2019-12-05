package cloud

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	metadataURL = "http://169.254.169.254/metadata/instance"
)

type ComputeMetadata struct {
	AZEnvironment        string `json:"azEnvironment,omitempty"`
	SKU                  string `json:"sku,omitempty"`
	Name                 string `json:"name,omitempty"`
	Zone                 string `json:"zone,omitempty"`
	VMSize               string `json:"vmSize,omitempty"`
	OSType               string `json:"osType,omitempty"`
	Location             string `json:"location,omitempty"`
	PlatformFaultDomain  string `json:"platformFaultDomain,omitempty"`
	PlatformUpdateDomain string `json:"platformUpdateDomain,omitempty"`
	ResourceGroupName    string `json:"resourceGroupName,omitempty"`
	VMScaleSetName       string `json:"vmScaleSetName,omitempty"`
	SubscriptionID       string `json:"subscriptionId,omitempty"`
}

type NetworkMetadata struct {
	Interface []NetworkInterface `json:"interface"`
}

type NetworkInterface struct {
	IPv4       NetworkData `json:"ipv4"`
	IPv6       NetworkData `json:"ipv6"`
	MACAddress string      `json:"macAddress"`
}

type NetworkData struct {
	IPAddress []IPAddress `json:"ipAddress"`
	Subnet    []Subnet    `json:"subnet"`
}

type IPAddress struct {
	PrivateIPAddress string `json:"privateIpAddress"`
	PublicIPAddress  string `json:"publicIpAddress"`
}

type Subnet struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

// InstanceMetadata represents instance information.
type InstanceMetadata struct {
	Compute *ComputeMetadata `json:"compute,omitempty"`
	Network *NetworkMetadata `json:"network,omitempty"`
}

type AzureCloudInfo struct {
	url      string
	metadata *InstanceMetadata
}

func NewAzureCloudInfo(url string) (CloudInfo, error) {
	if url == "" {
		url = metadataURL
	}
	az := &AzureCloudInfo{
		url: url,
	}
	_, err := az.getInstanceMetadata()
	if err != nil {
		return nil, err
	}
	return az, nil
}

// Get the main IP address (first IP on the main interface).
func (a *AzureCloudInfo) GetMainIPv4Address() (string, error) {
	return a.getIPv4Address(0)
}

// Get the IP address (second IP on the main interface) assigned to the pod.
func (a *AzureCloudInfo) GetPodIPv4Address() (string, error) {
	return a.getIPv4Address(1)
}

func (a *AzureCloudInfo) getIPv4Address(n int) (string, error) {
	metadata, err := a.getInstanceMetadata()
	if err != nil {
		return "", err
	}
	if metadata.Network == nil ||
		len(metadata.Network.Interface) < 1 ||
		len(metadata.Network.Interface[0].IPv4.IPAddress) < n+1 {
		return "", fmt.Errorf("no IP address found (requested %d)", n)
	}
	return metadata.Network.Interface[0].IPv4.IPAddress[n].PrivateIPAddress, nil
}

func (a *AzureCloudInfo) getInstanceMetadata() (*InstanceMetadata, error) {
	if a.metadata != nil {
		return a.metadata, nil
	}
	req, err := http.NewRequest("GET", a.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Metadata", "True")
	req.Header.Add("User-Agent", "elotl/itzo")
	q := req.URL.Query()
	q.Add("format", "json")
	q.Add("api-version", "2019-03-11")
	req.URL.RawQuery = q.Encode()
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"getting instance metadata: got response %q", resp.Status)
	}
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	metadata := InstanceMetadata{}
	err = json.Unmarshal(buf, &metadata)
	if err != nil {
		return nil, err
	}
	a.metadata = &metadata
	return a.metadata, nil
}
