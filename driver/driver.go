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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	driverName   = "com.tuxera.csi.moosefs"
	version      = "dev"
	gitTreeState = "not a git tree"
	commit       = ""
)

// CSIDriver implements the following CSI interfaces:
//
//   csi.IdentityServer
//   csi.ControllerServer
//   csi.NodeServer
//
type CSIDriver struct {
	endpoint string
	topology string
	nodeID   string
	// AWS
	awsAccessKey    string
	awsSecret       string
	awsSessionToken string
	awsRegion       string
	// Existing endpoint
	mfsEP string

	srv     *grpc.Server
	log     *logrus.Entry
	mounter Mounter
}

// NewCSIDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managaing Moosefs Storage
func NewCSIDriver(ep, topo, awsAccessKeyID, awsSecret, awsSessionToken, awsRegion, mfsEP string) (*CSIDriver, error) {
	nodeID := ep // TODO(Anoop): get hostname from EP

	return &CSIDriver{
		endpoint:        ep,
		topology:        topo,
		nodeID:          nodeID,
		awsAccessKey:    awsAccessKeyID,
		awsSecret:       awsSecret,
		awsSessionToken: awsSessionToken,
		awsRegion:       awsRegion,
		mfsEP:           mfsEP,
		mounter:         &mounter{},
		log: logrus.New().WithFields(logrus.Fields{
			"node_id": nodeID,
		}),
	}, nil
}

// Run starts the CSI plugin by communication over the given endpoint
func (d *CSIDriver) Run() error {
	u, err := url.Parse(d.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %q", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		addr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	} else {
		// remove the socket if it's already there. This can happen if we
		// deploy a new version and the socket was created from the old running
		// plugin.
		d.log.WithField("socket", addr).Info("removing socket")
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", addr, err)
		}
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			d.log.WithError(err).WithField("method", info.FullMethod).Error("method failed")
		}
		return resp, err
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	d.log.WithField("addr", addr).Info("server started")
	return d.srv.Serve(listener)
}

// Stop stops the plugin
func (d *CSIDriver) Stop() {
	d.log.Info("server stopped")
	d.srv.Stop()
}

func GetVersion() string {
	return version
}

// GetCommit returns the current commit hash value, as inserted at build time.
func GetCommit() string {
	return commit
}

// GetTreeState returns the current state of git tree, either "clean" or
// "dirty".
func GetTreeState() string {
	return gitTreeState
}
