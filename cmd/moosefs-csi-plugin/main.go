/*
   Copyright (c) 2023 Saglabs SA. All Rights Reserved.

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

	"github.com/xandrus/moosefs-csi/driver"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		mode             = flag.String("mode", "value", "")
		csiEndpoint      = flag.String("csi-endpoint", "unix:///var/lib/csi/sockets/pluginproxy/csi.sock", "CSI endpoint")
		mfsmaster        = flag.String("master-host", "mfsmaster", "MooseFS endpoint to use (already provisioned cluster), e.g. 192.168.75.201")
		mfsmaster_port   = flag.Int("master-port", 9421, "")
		nodeId           = flag.String("node-id", "", "")
		rootDir          = flag.String("root-dir", "/", "")
		pluginDataDir    = flag.String("plugin-data-dir", "/", "")
		mountPointsCount = flag.Int("mount-points-count", 1, "")
		mfsMountOptions  = flag.String("mfs-mount-options", "", "extra options passed to mfsmount command for example mfsmd5pass=MD5")
		sanityTestRun    = flag.Bool("sanity-test-run", false, "")
		logLevel         = flag.Int("log-level", 5, "")
		mfsLog           = flag.Bool("mfs-logging", true, "")
	)
	flag.Parse()

	driver.Init(*sanityTestRun, *logLevel, *mfsLog)

	if *sanityTestRun {
		log.Infof("=============== SANITY TEST ===============")
	}
	// this won't be logged to mfs log file
	log.Infof("Starting new service (mode: %s; master-host: %s; master-port: %d; mfs-mount-options %s; node-id: %s; root-dir: %s; plugin-data-dir: %s)",
		*mode, *mfsmaster, *mfsmaster_port, *mfsMountOptions, *nodeId, *rootDir, *pluginDataDir)

	var srv driver.Service
	var err error
	switch *mode {
	case "node":
		srv, err = driver.NewNodeService(*mfsmaster, *mfsmaster_port, *rootDir, *pluginDataDir, *nodeId, *mfsMountOptions, *mountPointsCount)
		if err != nil {
			log.Errorf("main - couldn't create node service. Error: %s", err.Error())
			return
		}
	case "controller":
		srv, err = driver.NewControllerService(*mfsmaster, *mfsmaster_port, *rootDir, *pluginDataDir, *mfsMountOptions)
		if err != nil {
			log.Errorf("main - couldn't create controller service. Error: %s", err.Error())
			return
		}
	default:
		log.Errorf("main - unrecognized mode = %s", *mode)
		return
	}

	if err = driver.StartService(&srv, *mode, *csiEndpoint); err != nil {
		log.Errorf("main - couldn't start service %s", err.Error())
	}
}
