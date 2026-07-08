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
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestServeWithGracefulShutdownOnSIGTERM verifies that
// serveWithGracefulShutdown stops its gRPC server and returns within a
// reasonable grace period when the process receives SIGTERM. This
// guards the graceful-shutdown contract (issue #32 / AGENTS.md item D):
// on SIGTERM/SIGINT the plugin must stop accepting new RPCs, let
// in-flight RPCs drain, and exit -- rather than hanging until kubelet
// SIGKILLs it after the termination grace period. Uses a bare
// grpc.Server (no CSI service / MooseFS master) so the test runs in
// pure unit-test isolation.
func TestServeWithGracefulShutdownOnSIGTERM(t *testing.T) {
	Init(false, 5, false, false)

	sockDir, err := os.MkdirTemp("/tmp", "mfsgrpc")
	if err != nil {
		t.Fatalf("os.MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "csi.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	srv := CreategRPCServer()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- serveWithGracefulShutdown(srv, ln)
	}()

	// Wait until the server is actually listening on the socket before
	// signalling, otherwise GracefulStop could race ahead of Serve.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fi, err := os.Stat(sockPath); err == nil && fi.Size() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill SIGTERM: %v", err)
	}

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("serveWithGracefulShutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveWithGracefulShutdown did not return within 5s of SIGTERM")
	}
}