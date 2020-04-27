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

package cloud

import (
    "fmt"
    "log"
    "net"
    "net/http"
    "time"

    "cloud.google.com/go/compute/metadata"
)

const (
    metadataAliasCIDR = "instance/network-interfaces/0/ip-aliases/0"
    metadataIPv4 = "instance/network-interfaces/0/ip"
)

type itzoTransport struct {
    userAgent   string
    base        http.RoundTripper
}

func (t itzoTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    req.Header.Set("User-Agent", t.userAgent)
    return t.base.RoundTrip(req)
}

type GceCloudInfo struct {
    metadata *metadata.Client
}

func NewGceCloudInfo() (CloudInfo, error) {
    c := &http.Client{
        Timeout: 10 * time.Second,
        Transport: itzoTransport{
            userAgent: "elotl/itzo",
            base: http.DefaultTransport,
        }
    }
    metadataClient := metadata.NewClient(c)
    return &GceCloudInfo{
        metadata: metadataClient,
    }, nil
}

func (g *GceCloudInfo) GetMainIPv4Address() (string, error) {
    if !metadata.OnGCE() {
        return "", fmt.Errorf("instance is not running inside GCE, could not retrieve Node IP")
    }

    addr, err := g.metadata.Get(metadataIPv4)
    if err != nil {
        return "", err
    }

    return addr, nil
}

func (g *GceCloudInfo) GetPodIPv4Address() (string, error) {
    if !metadata.OnGCE() {
        return "", fmt.Errorf("instance is not running inside GCE, could not retrieve Pod IP")
    }

    cidr, err := g.metadata.Get(metadataAliasCIDR)
    if err != nil {
        return "", err
    }

    // we dont want the cidr only the ip if the mask is 32 bits if larger return 
    // returns us an IP addr, an IP network (IP + submask), and err
    _, ipNet, err := net.ParseCIDR(cidr)
    if err != nil {
        return "", err
    }
    _, bits := ipNet.Mask.Size()
    if bits != 32 {
        return "", fmt.Errorf("cannot get pod ip over an ip range")
    }

    addr := ipNet.IP.String()
    return addr, nil
}
