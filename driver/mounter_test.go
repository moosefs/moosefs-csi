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
	"reflect"
	"testing"
)

func TestMounterCommandPlain(t *testing.T) {
	m := &Mounter{}
	cmd := m.command("mount", "-t", "moosefs", "src", "dst")

	if cmd.Path == "" {
		t.Fatalf("expected resolved path, got empty")
	}
	// Args[0] is the resolved program name/path, rest should be untouched.
	if !reflect.DeepEqual(cmd.Args[1:], []string{"-t", "moosefs", "src", "dst"}) {
		t.Errorf("unexpected args for non-host-namespace command: %v", cmd.Args)
	}
}

func TestMounterCommandHostNamespace(t *testing.T) {
	m := &Mounter{UseHostNamespace: true}
	cmd := m.command("mount", "-t", "moosefs", "src", "dst")

	wantArgs := []string{
		"--target", "1",
		"--mount",
		"--pid",
		"--",
		"mount",
		"-t", "moosefs", "src", "dst",
	}
	if !reflect.DeepEqual(cmd.Args[1:], wantArgs) {
		t.Errorf("unexpected args for host-namespace command: got %v, want %v", cmd.Args[1:], wantArgs)
	}
}

func TestLazyUMountArgsPlain(t *testing.T) {
	m := &Mounter{}
	cmd := m.command(umountCmd, "-l", "/mnt/node1")
	if !reflect.DeepEqual(cmd.Args[1:], []string{"-l", "/mnt/node1"}) {
		t.Errorf("LazyUMount plain args: got %v", cmd.Args[1:])
	}
}

func TestLazyUMountArgsHostNamespace(t *testing.T) {
	m := &Mounter{UseHostNamespace: true}
	cmd := m.command(umountCmd, "-l", "/mnt/node1")
	wantArgs := []string{
		"--target", "1",
		"--mount",
		"--pid",
		"--",
		umountCmd,
		"-l", "/mnt/node1",
	}
	if !reflect.DeepEqual(cmd.Args[1:], wantArgs) {
		t.Errorf("LazyUMount host-namespace args: got %v, want %v", cmd.Args[1:], wantArgs)
	}
}
