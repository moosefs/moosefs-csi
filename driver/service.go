/*
   Copyright 2021 Tappest sp. z o.o. All Rights Reserved.

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
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	//	"github.com/kubernetes-csi/csi-test/v3/driver"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	driverName    = "moosefs.csi.tappest.com"
	driverVersion = "0.9.1-dev"
)

type Service interface{}

var SanityTestRun bool
var MfsLog bool
var log logrus.Logger

func Init(sanityTestRun bool, logLevel int, mfsLog bool) error {
	log = *logrus.New()
	SanityTestRun = sanityTestRun
	log.SetLevel(logrus.Level(logLevel))
	MfsLog = mfsLog
	return nil
}

func StartService(service *Service, mode, csiEndpoint string) error {
	log.Infof("StartService - endpoint %s", csiEndpoint)
	gRPCServer := CreategRPCServer()
	listener, err := CreateListener(csiEndpoint)
	if err != nil {
		return err
	}
	csi.RegisterIdentityServer(gRPCServer, &IdentityService{})

	switch (*service).(type) {
	case *NodeService:
		log.Infof("StartService - Registering node service")
		csi.RegisterNodeServer(gRPCServer, (*service).(csi.NodeServer))
	case *ControllerService:
		log.Infof("StartService - Registering controller service")
		csi.RegisterControllerServer(gRPCServer, (*service).(csi.ControllerServer))
	default:
		return fmt.Errorf("StartService: Unrecognized service type: %T", service)
	}

	log.Info("StartService - Starting to serve!")
	err = gRPCServer.Serve(listener)
	if err != nil {
		return err
	}
	log.Info("StartService - gRPCServer stopped without an error!")
	return nil
}

// CreateListener create listener ready for communication over given csi endpoint
func CreateListener(csiEndpoint string) (net.Listener, error) {
	log.Infof("CreateListener - endpoint %s", csiEndpoint)

	u, err := url.Parse(csiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("CreateListener - Unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return nil, fmt.Errorf("CreateListener - Currently only unix domain sockets are supported, have: %s", u.Scheme)
	} else {
		// remove the socket if it's already there. This can happen if we
		// deploy a new version and the socket was created from the old running
		// plugin.
		log.Infof("CreateListener - Removing socket %s", addr)
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("CreateListener - Failed to remove unix domain socket file %s, error: %s", addr, err)
		}
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return nil, fmt.Errorf("CreateListener - Failed to listen: %v", err)
	}

	return listener, nil
}

func CreategRPCServer() *grpc.Server {
	log.Info("CreategRPCServer")
	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			stat, rpcErr := status.FromError(err)
			if rpcErr {
				log.Errorf("rpc error: %s - %s", stat.Code(), stat.Message())
			} else {
				log.Errorf("unexpected error type - %s", err.Error())
			}
		}
		return resp, err
	}
	return grpc.NewServer(grpc.UnaryInterceptor(errHandler))
}
