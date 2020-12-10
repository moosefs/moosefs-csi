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
package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	driverName    = "moosefs.csi.tappest.com"
	driverVersion = "0.5-dev"
)

type PluginServiceInterface interface {
	Register(*grpc.Server)
}

type PluginService struct {
	PluginServiceInterface
	grpcSrv *grpc.Server
}

var _ PluginServiceInterface = &PluginService{}

// Run starts the CSI plugin by communication over the given endpoint
func (srv *PluginService) Run(csiEndpoint string) error {
	log.Infof("PluginService::Run")

	u, err := url.Parse(csiEndpoint)
	if err != nil {
		return fmt.Errorf("PluginService::Run unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("PluginService::Run currently only unix domain sockets are supported, have: %s", u.Scheme)
	} else {
		// remove the socket if it's already there. This can happen if we
		// deploy a new version and the socket was created from the old running
		// plugin.
		log.WithField("socket", addr).Info("removing socket")
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("PluginService::Run failed to remove unix domain socket file %s, error: %s", addr, err)
		}
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("PluginService::Run failed to listen: %v", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			log.WithError(err).WithField("method", info.FullMethod).Error("method failed")
		}
		return resp, err
	}

	srv.grpcSrv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	srv.Register(srv.grpcSrv)

	return srv.grpcSrv.Serve(listener)
}

// stops the plugin
func (srv *PluginService) Stop() {
	log.Infof("PluginService::Stop")
	srv.grpcSrv.Stop()
}
