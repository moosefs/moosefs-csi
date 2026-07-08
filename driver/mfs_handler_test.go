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
	"os"
	"syscall"
	"testing"
)

func TestMfsSourceURI(t *testing.T) {
	cases := []struct {
		name     string
		rootPath string
		subPath  string
		want     string
	}{
		{
			name:     "root mount, root path /",
			rootPath: "/",
			subPath:  "",
			want:     "mfsmaster:9421:/",
		},
		{
			name:     "root mount, custom root path preserved as-is",
			rootPath: "/foo/",
			subPath:  "",
			want:     "mfsmaster:9421:/foo/",
		},
		{
			name:     "staged volume under root /",
			rootPath: "/",
			subPath:  "pv_data/volumes/pvc-1234",
			want:     "mfsmaster:9421:/pv_data/volumes/pvc-1234",
		},
		{
			name:     "staged volume under nested root path",
			rootPath: "/k8s-root",
			subPath:  "pv_data/volumes/pvc-1234",
			want:     "mfsmaster:9421:/k8s-root/pv_data/volumes/pvc-1234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewMfsHandler("mfsmaster", 9421, tc.rootPath, "pv_data", "node1", "")
			got := h.mfsSourceURI(tc.subPath)
			if got != tc.want {
				t.Errorf("mfsSourceURI(%q) = %q, want %q", tc.subPath, got, tc.want)
			}
		})
	}
}

func TestMfsPathToVolume(t *testing.T) {
	h := NewMfsHandler("mfsmaster", 9421, "/", "pv_data", "node1", "")
	got := h.MfsPathToVolume("pvc-1234")
	want := "pv_data/volumes/pvc-1234"
	if got != want {
		t.Errorf("MfsPathToVolume() = %q, want %q", got, want)
	}
}

func TestMountMfsVolumeStagedSourceMatchesMfsPathToVolume(t *testing.T) {
	// Regression guard for issue #32: NodeStageVolume must mount the
	// volume's actual MooseFS sub-path (rootPath + MfsPathToVolume),
	// not the pool-mount-local path used by the legacy bind-mount design.
	h := NewMfsHandler("mfsmaster", 9421, "/k8s-root", "pv_data", "node1", "")
	volumeId := "pvc-abcd"
	subPath := h.MfsPathToVolume(volumeId)
	got := h.mfsSourceURI(subPath)
	want := "mfsmaster:9421:/k8s-root/pv_data/volumes/pvc-abcd"
	if got != want {
		t.Errorf("mfsSourceURI(MfsPathToVolume(%q)) = %q, want %q", volumeId, got, want)
	}
}

func TestMergeMountOptionsAddsDefaults(t *testing.T) {
	// With no base and no user options, all defaults must be present.
	got := mergeMountOptions(nil, "")
	want := defaultMountOptions
	if !sameSet(got, want) {
		t.Errorf("mergeMountOptions(nil,\"\") = %v, want defaults %v", got, want)
	}
}

func TestMergeMountOptionsUserOverridesDefault(t *testing.T) {
	// A user-supplied key must win over the same default key.
	got := mergeMountOptions(nil, "mfsioretries=60,mfsmd5pass=abc")
	if !containsKV(got, "mfsioretries", "60") {
		t.Errorf("user mfsioretries=60 should override default; got %v", got)
	}
	if !containsKV(got, "mfsmd5pass", "abc") {
		t.Errorf("user mfsmd5pass=abc missing; got %v", got)
	}
	if containsKV(got, "mfsioretries", "30") {
		t.Errorf("default mfsioretries=30 should have been overridden; got %v", got)
	}
	// Untouched defaults must still be present.
	if !containsKey(got, "mfstimeout") || !containsKey(got, "allow_other") {
		t.Errorf("untouched defaults missing; got %v", got)
	}
}

func TestMergeMountOptionsBaseWinsOverUserAndDefault(t *testing.T) {
	// Volume-capability flags (base) take top precedence.
	got := mergeMountOptions([]string{"mfstimeout=120"}, "mfstimeout=90")
	if !containsKV(got, "mfstimeout", "120") {
		t.Errorf("base mfstimeout=120 should win; got %v", got)
	}
}

func TestMergeMountOptionsDedupBareFlag(t *testing.T) {
	// allow_other is a bare flag (no =value); it must not be duplicated.
	got := mergeMountOptions([]string{"allow_other"}, "allow_other")
	count := 0
	for _, o := range got {
		if o == "allow_other" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("allow_other appeared %d times, want 1; got %v", count, got)
	}
}

func TestMergeMountOptionsStagedGetsAuth(t *testing.T) {
	// Regression for the critical auth bug: MountMfsVolumeStaged must
	// include mfsmd5pass from mfsMountOptions even when the caller
	// (NodeStageVolume) only passes volume-capability flags.
	h := NewMfsHandler("mfsmaster", 9421, "/", "pv_data", "node1", "mfsmd5pass=secret")
	got := mergeMountOptions([]string{"ro"}, h.mfsMountOptions)
	if !containsKV(got, "mfsmd5pass", "secret") {
		t.Errorf("staged mount missing auth mfsmd5pass; got %v", got)
	}
	if !containsKey(got, "ro") {
		t.Errorf("base flag ro dropped; got %v", got)
	}
}

// sameSet compares two slices ignoring order.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := map[string]int{}
	for _, s := range a {
		ma[s]++
	}
	for _, s := range b {
		ma[s]--
		if ma[s] < 0 {
			return false
		}
	}
	return true
}

func containsKV(opts []string, k, v string) bool {
	target := k + "=" + v
	for _, o := range opts {
		if o == target {
			return true
		}
	}
	return false
}

func containsKey(opts []string, k string) bool {
	for _, o := range opts {
		key := o
		if i := indexByte(o, '='); i >= 0 {
			key = o[:i]
		}
		if key == k {
			return true
		}
	}
	return false
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func TestIsMountStaleENOTCONN(t *testing.T) {
	err := &os.PathError{Op: "stat", Path: "/mnt/x", Err: syscall.ENOTCONN}
	got, errOut := isStaleStatError(err)
	if errOut != nil {
		t.Fatalf("unexpected error: %v", errOut)
	}
	if !got {
		t.Errorf("ENOTCONN should be classified as stale")
	}
}

func TestIsMountStaleESTALE(t *testing.T) {
	err := &os.PathError{Op: "stat", Path: "/mnt/x", Err: syscall.ESTALE}
	got, errOut := isStaleStatError(err)
	if errOut != nil {
		t.Fatalf("unexpected error: %v", errOut)
	}
	if !got {
		t.Errorf("ESTALE should be classified as stale")
	}
}

func TestIsMountStaleHealthy(t *testing.T) {
	// A nil error (healthy mount) is not stale.
	got, err := isStaleStatError(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("nil error should not be classified as stale")
	}
}

func TestIsMountStaleOtherErrno(t *testing.T) {
	// An unrelated errno must surface as an error, not as stale.
	err := &os.PathError{Op: "stat", Path: "/mnt/x", Err: syscall.EACCES}
	got, errOut := isStaleStatError(err)
	if got {
		t.Errorf("EACCES should not be classified as stale")
	}
	if errOut == nil {
		t.Errorf("EACCES should propagate as an error")
	}
}

func TestIsMountStaleNonPathError(t *testing.T) {
	// A non-*os.PathError must propagate unchanged.
	in := syscall.EINVAL
	got, errOut := isStaleStatError(in)
	if got {
		t.Errorf("non-PathError should not be classified as stale")
	}
	if errOut == nil {
		t.Errorf("non-PathError should propagate as an error")
	}
}
