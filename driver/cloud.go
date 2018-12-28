/*
   Copyright 2018 Tuxera Oy. All Rights Reserved.

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

package driver

import (
	"errors"
	"os/exec"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
)

const (
	AWS   = "AWS"   // Amazon Web Services
	GCP   = "GCP"   // Google Cloud Platform
	AZURE = "AZURE" // Azure cloud Platform
)

// Topology for  MooseFS topology
type Topology struct {
	Master string
	Chunk  string
}

// CreateVolOutput abstractation for all cloud vendors
type CreateVolOutput struct {
	AWS        AWSCreateVolOutput
	Endpoint   string
	InstanceID string
}

// CreateVol - Generic for all cloud vendors
func CreateVol(volName string, d *Driver, volSize int64) (CreateVolOutput, error) {

	if d.topology == "" {
		return CreateVolOutput{}, errors.New("MooseFS topology cannot be empty")
	}

	if !verifyTopologyFormat(d.topology) {
		return CreateVolOutput{}, errors.New("Wrong MooseFS topology format")
	}

	topo := parseTopology(d.topology)
	if topo.Master == AWS && topo.Chunk == AWS {
		out, err := AWSCreateVol(volName, d, volSize)
		if err != nil {
			return CreateVolOutput{}, err
		}

		return CreateVolOutput{
			AWS:        out,
			Endpoint:   out.volID,
			InstanceID: *out.Ec2Res.Instances[0].InstanceId,
		}, nil
	} else {
		//TODO(anoop): No support yet
		return CreateVolOutput{}, errors.New("No support for topologies other than AWS yet")
	}

}

// DeleteVol - Generic for all cloud vendors
func DeleteVol(volName string, d *Driver) error {
	topo := parseTopology(d.topology)
	if topo.Master == AWS && topo.Chunk == AWS {

		if err := AWSDeleteVol(volName, d); err != nil {
			return err
		}

	} else {
		//TODO(anoop): No support yet
		return errors.New("No support for topologies other than AWS yet")
	}
	return nil
}

// ControllerPublishVol - Generic for all cloud vendors
func ControllerPublishVol(d *Driver, req *csi.ControllerPublishVolumeRequest) error {
	topo := parseTopology(d.topology)
	if topo.Master == AWS && topo.Chunk == AWS {
		if err := AWSControllerPublishVol(d, req); err != nil {
			return nil
		}
	} else {
		//TODO(anoop): No support yet
		return errors.New("No support for topologies other than AWS yet")
	}
	return nil
}

// ControllerUnPublishVol - generic
func ControllerUnPublishVol(d *Driver, req *csi.ControllerUnpublishVolumeRequest) error {
	// TODO(Anoop): check what needs to be done more
	// Moosefs being ditributed filesystem, nothing needed to be done
	// detachVol()
	return nil
}

// Validates the string format of the topology
// Valid formats: "master:AWS,chunk:GCP", "chunk:Azure,master:Aws"
func verifyTopologyFormat(topology string) bool {
	if !strings.ContainsAny(topology, ",") {
		return false
	}
	if !strings.ContainsAny(topology, "master:") {
		return false
	}
	if !strings.ContainsAny(topology, "chunk:") {
		return false
	}
	if !strings.ContainsAny(topology, AWS) && !strings.ContainsAny(topology, GCP) &&
		!strings.ContainsAny(topology, AZURE) {
		return false
	}
	return true
}

// Parses the topology string, ensure validatation before parsing
// E.g. "master:AWS,chunk:GCP", "chunk:Azure,master:Aws"
func parseTopology(topology string) *Topology {

	var master, chunk = AWS, AWS // Defaults to AWS

	t := strings.Split(topology, ",")
	for _, c := range t {
		m := strings.Split(c, ":")
		if m[0] == "master:" {
			master = m[1]
		} else {
			chunk = m[1]
		}
	}

	return &Topology{
		Master: master,
		Chunk:  chunk,
	}
}

/*GetPublicIP4K8s Get public IP of the given K8s cluster.
Note that, the k8s cluster must have egress enabled for this to work
*/
func GetPublicIP4K8s() string {
	var (
		URL1 = "ifconfig.me"
		URL2 = "ifconfig.co"
	)
	curlFunc := func(url string, ch chan string) {
		out, _ := exec.Command("curl", "-s", url).CombinedOutput()
		ch <- strings.TrimSpace(string(out))
	}
	ip1 := make(chan string)
	ip2 := make(chan string)
	go curlFunc(URL1, ip1)
	go curlFunc(URL2, ip2)

	select {
	case ip := <-ip1:
		return ip
	case ip := <-ip2:
		return ip
	}

}
