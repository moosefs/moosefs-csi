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

package main

import (
	"flag"
	"log"

	"github.com/moosefs/moosefs-csi/driver"
)

func main() {
	var (
		endpoint        = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/com.tuxera.csi.moosefs/csi.sock", "CSI endpoint")
		topo            = flag.String("topology", "master:AWS,chunk:AWS", "MooseFS cluster topology")
		awsAccessKeyID  = flag.String("aws-access", "", "AWS Access key Id")
		awsSessionToken = flag.String("aws-session", "", "AWS Session token")
		awsSecret       = flag.String("aws-secret", "", "AWS Secret Access key")
		awsRegion       = flag.String("aws-region", "eu-west-1", "AWS region where to deploy")
	)
	flag.Parse()

	drv, err := driver.NewDriver(*endpoint, *topo, *awsAccessKeyID, *awsSecret, *awsSessionToken, *awsRegion)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}
