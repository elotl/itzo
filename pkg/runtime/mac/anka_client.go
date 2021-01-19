package mac

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

const (
	listVMTemplatesPath = "/registry/v2/vm"
)

type RegistryClient interface {
	GetVMTemplate(vmTemplateID string) (string, error)
}

type AnkaRegistryClient struct {}

func NewAnkaRegistryClient() *AnkaRegistryClient {
	return &AnkaRegistryClient{}
}

func parseImageUrl(imageUrl string) (vmId, fullUrl string) {
	// expected format is <url>:<vm-id>
	// or <host>:<port>:<vm-id>
	// or <proto>:<host>:<port>:<vm-id>
	ind := strings.LastIndex(imageUrl, "/")
	vmId = imageUrl[ind+1:]
	urlPart := imageUrl[:ind]
	baseUrl := urlPart + listVMTemplatesPath + "?id=" + vmId
	parsedUrl, err := url.Parse(baseUrl)
	if err != nil {
		glog.Errorf("parsing %s failed with %v", fullUrl, err)
		return vmId, ""
	}
	fullUrl = parsedUrl.String()
	// we want to always get an url with proto as an output of this function.
	if !strings.HasPrefix(fullUrl, "http://") {
		return vmId, "http://" + fullUrl
	}
	return vmId, fullUrl

}

func (cc *AnkaRegistryClient) GetVMTemplate(vmTemplateUrl string) (string, error) {
	vmId, templateUrl  := parseImageUrl(vmTemplateUrl)
	if templateUrl == "" {
		return "", fmt.Errorf("cannot parse %s", vmTemplateUrl)
	}
	client := http.Client{}
	resp, err := client.Get(templateUrl)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("cannot find vm template on %s", templateUrl)
	}
	var respBody VMRespBase
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(respBytes, &respBody)
	if err != nil {
		return "", err
	}
	if respBody.Status != AnkaStatusOK {
		return "", fmt.Errorf("cannot find vm template on url %s", templateUrl)
	}
	return vmId, nil
}
