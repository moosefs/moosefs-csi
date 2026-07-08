/*
   Copyright (c) 2026 Saglabs SA. All Rights Reserved.

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
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	driverName    = "csi.moosefs.com"
	driverVersion = "1.0.0"
)

type Service interface{}

// Stopper is implemented by services that own resources which must be
// released on graceful shutdown. Currently only *NodeService implements
// it, to unmount the shared per-node pool mount(s) on SIGTERM/SIGINT so
// that open MooseFS master sessions are closed cleanly. Per-volume
// staging mounts are intentionally NOT touched here: they are created
// in the host namespace (see MountMfsVolumeStaged / HostNamespaceMount)
// precisely so they outlive the csi-moosefs-node container and keep
// serving application pods across plugin restarts.
type Stopper interface {
	Stop()
}

var SanityTestRun bool
var MfsLog bool
var log logrus.Logger

// HostNamespaceMount controls whether per-volume staging mounts are executed
// in the host's mount/PID namespaces (via nsenter) rather than the
// csi-moosefs-node container's own namespace. This decouples the mfsmount
// FUSE daemon lifetime from the plugin container's lifetime, fixing mounts
// becoming unusable ("mount through procfd", ENOTCONN) whenever the plugin
// container restarts. Requires hostPID: true on the node DaemonSet.
// See: https://github.com/moosefs/moosefs-csi/issues/32
var HostNamespaceMount bool

func Init(sanityTestRun bool, logLevel int, mfsLog bool, hostNamespaceMount bool) error {
	log = *logrus.New()
	SanityTestRun = sanityTestRun
	log.SetLevel(logrus.Level(logLevel))
	MfsLog = mfsLog
	HostNamespaceMount = hostNamespaceMount
	return nil
}

// StartService starts the gRPC server and blocks until the server stops.
// On SIGTERM/SIGINT it performs a graceful gRPC shutdown
// (GracefulStop) and, when the underlying service implements Stopper,
// runs service-level cleanup (e.g. unmounting the node-plugin pool
// mount) before returning. This avoids leaving dangling master sessions
// when kubelet evicts the csi-moosefs-node pod. Per-volume staging
// mounts are never torn down here (see Stopper docs).
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

	// Graceful shutdown: stop accepting new RPCs on SIGTERM/SIGINT,
	// let in-flight RPCs drain, then run service cleanup.
	if err := serveWithGracefulShutdown(gRPCServer, listener); err != nil {
		return err
	}

	// Service-level cleanup (e.g. unmount pool mount on the node plugin).
	if stopper, ok := (*service).(Stopper); ok {
		log.Infof("StartService - running service cleanup (mode: %s)", mode)
		stopper.Stop()
	}

	log.Info("StartService - gRPCServer stopped without an error!")
	return nil
}

// serveWithGracefulShutdown runs srv.Serve(ln) and arranges for a
// graceful shutdown (srv.GracefulStop) when the process receives
// SIGTERM or SIGINT. It returns the error from Serve. Extracted from
// StartService so the signal-handling contract is unit-testable with a
// bare grpc.Server (no CSI service / MooseFS master required).
func serveWithGracefulShutdown(srv *grpc.Server, ln net.Listener) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	stopCh := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			log.Infof("serveWithGracefulShutdown - received %s, gracefully stopping gRPC server", sig)
			srv.GracefulStop()
		case <-stopCh:
		}
	}()

	log.Info("serveWithGracefulShutdown - starting to serve")
	err := srv.Serve(ln)
	close(stopCh)
	return err
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
