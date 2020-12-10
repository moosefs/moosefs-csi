/*
   Copyright 2019 Tuxera Oy. All Rights Reserved.

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
	"fmt"
	"path"

	"github.com/moosefs/moosefs-csi/driver"
	log "github.com/sirupsen/logrus"
)

var Mfslog *driver.MfsLog

func main() {
	var (
		mode             = flag.String("mode", "value", "")
		csiEndpoint      = flag.String("csi-endpoint", "unix:///var/lib/kubelet/plugins/com.tuxera.csi.moosefs/csi.sock", "CSI endpoint")
		mfsmaster        = flag.String("master-host", "mfsmaster", "MooseFS endpoint to use (already provisioned cluster), e.g. 192.168.75.201")
		nodeID           = flag.String("node-id", "", "")
		rootDir          = flag.String("root-dir", "/", "")
		pluginDataDir    = flag.String("plugin-data-dir", "/.persistent_volumes", "")
		mountPointsCount = flag.Int("mount-points-count", 2, "")
	)
	flag.Parse()

	Mfslog = driver.MakeMfsLog(
		path.Join(fmt.Sprintf("/mnt/mount_node_%s_%02d", *nodeID, 0), *pluginDataDir,
			fmt.Sprintf("logz_%s", *nodeID)), false)

	var srv driver.PluginService

	switch *mode {
	case "node":
		srv_, err := driver.NewNodeService(*mfsmaster, *rootDir, *pluginDataDir, *nodeID, *mountPointsCount)
		if err != nil {
			panic(err)
		}
		srv = driver.PluginService{PluginServiceInterface: srv_}

	case "controller":
		srv_, err := driver.NewControllerService(*mfsmaster, *rootDir, *pluginDataDir)
		if err != nil {
			panic(err)
		}
		srv = driver.PluginService{PluginServiceInterface: srv_}

	default:
		log.Fatalf("main -- unrecognized --mode=%s", *mode)
		return
	}

	if err := srv.Run(*csiEndpoint); err != nil {
		//
		Mfslog.Log("teraz fatal")
		//
		log.Fatalln(err)
	}
	//
	Mfslog.Log("zaraz stop")
	//
	srv.Stop()
}
