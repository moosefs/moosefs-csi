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

/*
 * courtesy: https://github.com/digitalocean/csi-digitalocean/blob/master/driver/mounter.go
 */

package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type MounterInterface interface {
	// Mount a volume
	Mount(sourcePath string, destPath, mountType string, opts ...string) error

	// Unmount a volume
	UMount(destPath string) error

	// Verify mount
	IsMounted(destPath string) (bool, error)
}

type Mounter struct {
	MounterInterface

	// UseHostNamespace, when true, executes the mount/umount/findmnt
	// commands inside the host's mount and PID namespaces (via nsenter)
	// instead of the csi-moosefs-node container's own namespace.
	//
	// This is required for any mount whose lifetime must outlive the
	// (ephemeral) csi-moosefs-node container -- most notably the per-volume
	// staging mounts created in NodeStageVolume. Without this, the
	// underlying mfsmount FUSE daemon is a child process of the plugin
	// container and dies whenever that container restarts (image bump,
	// crash, eviction, ...), leaving every bind-mount derived from it
	// pointing at an orphaned/deleted dentry ("mount through procfd: no
	// such file or directory") or resulting in ENOTCONN for already-open
	// files.
	//
	// See: https://github.com/moosefs/moosefs-csi/issues/32
	//      https://github.com/moosefs/moosefs-csi/issues/30
	//      https://github.com/moosefs/moosefs-csi/issues/14
	//
	// Requires the csi-moosefs-node DaemonSet to run with hostPID: true so
	// that "nsenter --target 1" resolves to the real host init process, and
	// requires mount/umount/findmnt to be present on the *host* filesystem
	// (standard on any util-linux based distribution).
	UseHostNamespace bool
}

var _ MounterInterface = &Mounter{}

const (
	nsenterCmd = "nsenter"
)

// command builds an exec.Cmd for name+args, transparently routing it through
// nsenter into the host's mount and PID namespaces when UseHostNamespace is
// set. hostPID: true on the pod spec makes PID 1 (as seen by this container)
// the real host init process, which is what "--target 1" attaches to.
//
// When detaching is requested (detaching=true, used for the long-lived
// mfsmount daemons spawned by staging/pool mounts), the command is wrapped
// in "setsid --fork" so the spawned mfsmount daemon:
//   - starts a new session/process group (immune to SIGTERM/SIGKILL sent
//     to the csi-moosefs-node container's process group on shutdown), and
//   - is reparented to host PID 1 (init) once the transient nsenter shim
//     exits, so its lifetime is decoupled from the container.
//
// This is what makes published volumes survive csi-moosefs-node restarts
// (image bumps, crashes, evictions) without leaving stale FUSE mounts that
// return ENOTCONN to every I/O. See issue #32.
//
// NOTE on cgroups: nsenter cannot move the child into the host's root
// cgroup on all init systems, so the daemon may still be accounted under
// the container's cgroup for resource-limit purposes. On containerd/K8s
// this does NOT cause the daemon to be killed on container stop because
// the kill signal is sent to the container's PID namespace only, and the
// daemon has already escaped that namespace via --pid. setsid further
// protects against process-group-wide signals.
func (m *Mounter) command(name string, args ...string) *exec.Cmd {
	return m.commandWithDetach(name, false, args...)
}

// commandDetaching is like command but wraps the host-namespace invocation
// in setsid --fork so long-lived daemons (mfsmount) survive the container.
// Has no effect when UseHostNamespace is false (container-namespace mounts
// are inherently tied to the container's lifetime).
func (m *Mounter) commandDetaching(name string, args ...string) *exec.Cmd {
	return m.commandWithDetach(name, true, args...)
}

func (m *Mounter) commandWithDetach(name string, detaching bool, args ...string) *exec.Cmd {
	if !m.UseHostNamespace {
		return exec.Command(name, args...)
	}
	// nsenter into the host's mount + PID namespaces. hostPID: true on the
	// pod spec makes PID 1 (as seen by this container) the real host init
	// process, which is what "--target 1" attaches to.
	nsenterArgs := []string{"--target", "1", "--mount", "--pid", "--"}
	if detaching {
		// setsid --fork starts a new session in the host PID namespace so
		// the spawned mfsmount daemon is reparented to host init (PID 1)
		// once the transient nsenter/mount shim exits, and is immune to
		// process-group-wide signals sent to the csi-moosefs-node
		// container. This decouples the FUSE daemon's lifetime from the
		// container's. See issue #32.
		nsenterArgs = append(nsenterArgs, "setsid", "--fork")
	}
	nsenterArgs = append(nsenterArgs, name)
	nsenterArgs = append(nsenterArgs, args...)
	return exec.Command(nsenterCmd, nsenterArgs...)
}

type findmntResponse struct {
	FileSystems []fileSystem `json:"filesystems"`
}

type fileSystem struct {
	Target      string `json:"target"`
	Propagation string `json:"propagation"`
	FsType      string `json:"fstype"`
	Options     string `json:"options"`
}

const (
	mountCmd   = "mount"
	umountCmd  = "umount"
	findmntCmd = "findmnt"
	newDirMode = 0750
)

func (m *Mounter) Mount(sourcePath, destPath, mountType string, opts ...string) error {
	mountArgs := []string{}
	if sourcePath == "" {
		return errors.New("Mounter::Mount -- sourcePath must be provided")
	}

	if destPath == "" {
		return errors.New("Mounter::Mount -- Destination path must be provided")
	}

	mountArgs = append(mountArgs, "-t", mountType)
	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}

	mountArgs = append(mountArgs, sourcePath)
	mountArgs = append(mountArgs, destPath)

	// NOTE: directory creation intentionally happens in the container's own
	// namespace (not nsentered), relying on destPath being part of a path
	// already bind-mounted into this container with Bidirectional/shared
	// propagation (e.g. under /var/lib/kubelet). Because that subtree is a
	// shared peer group, a directory created here is immediately visible on
	// the host (and vice versa), so the subsequent (possibly nsentered)
	// mount call below can resolve destPath correctly in the host mount
	// namespace as well.
	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(destPath, newDirMode)
	if err != nil {
		return err
	}
	out, err := m.command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::Mount -- mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}
	return nil
}

// MountDetaching is like Mount but, when UseHostNamespace is set, spawns the
// mount via setsid --fork so the resulting FUSE daemon (mfsmount) is
// reparented to host init and survives this container's lifetime. Used for
// staging and pool mounts whose FUSE daemon must outlive the
// csi-moosefs-node pod (issue #32). For container-namespace mounts it is
// equivalent to Mount.
func (m *Mounter) MountDetaching(sourcePath, destPath, mountType string, opts ...string) error {
	mountArgs := []string{}
	if sourcePath == "" {
		return errors.New("Mounter::MountDetaching -- sourcePath must be provided")
	}
	if destPath == "" {
		return errors.New("Mounter::MountDetaching -- destPath must be provided")
	}
	mountArgs = append(mountArgs, "-t", mountType)
	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}
	mountArgs = append(mountArgs, sourcePath)
	mountArgs = append(mountArgs, destPath)

	if err := os.MkdirAll(destPath, newDirMode); err != nil {
		return err
	}
	out, err := m.commandDetaching(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::MountDetaching -- mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}
	return nil
}

func (m *Mounter) UMount(destPath string) error {
	umountArgs := []string{}

	if destPath == "" {
		return errors.New("Mounter::UMount -- Destination path must be provided")
	}
	// todo(ad): sprawdzanie czy istnieje katalog
	umountArgs = append(umountArgs, destPath)

	out, err := m.command(umountCmd, umountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::UMount -- mounting failed: %v cmd: '%s %s' output: %q",
			err, umountCmd, strings.Join(umountArgs, " "), string(out))
	}

	return nil
}

// LazyUMount forcibly detaches the mount at destPath using "umount -l".
// This is the recovery path for stale FUSE mounts left behind after a
// MooseFS master outage, network partition, or node reboot: such mounts
// are still listed by findmnt (so IsMounted returns true) but any I/O
// fails with ENOTCONN ("Transport endpoint is not connected"). A plain
// umount also fails on these; only a lazy detach clears the dentry so a
// fresh mfsmount can reclaim the path.
//
// When UseHostNamespace is set the command is routed through nsenter so
// the detach happens in the host mount namespace (where the stale FUSE
// mount actually lives for staging mounts created via
// MountMfsVolumeStaged).
func (m *Mounter) LazyUMount(destPath string) error {
	if destPath == "" {
		return errors.New("Mounter::LazyUMount -- Destination path must be provided")
	}
	out, err := m.command(umountCmd, "-l", destPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::LazyUMount -- lazy unmount failed: %v cmd: '%s -l %s' output: %q",
			err, umountCmd, destPath, string(out))
	}
	return nil
}

// IsMountStale reports whether the filesystem entry at path is a stale
// FUSE mount -- i.e. the kernel still has a mount record (so findmnt /
// IsMounted reports it as mounted) but the backing FUSE daemon is gone
// or unreachable, making every I/O fail with ENOTCONN ("Transport
// endpoint is not connected") or ESTALE. This complements IsMounted:
// IsMounted answers "is there a mount record?" while IsMountStale
// answers "is that mount record actually usable?". A mount that is
// both IsMounted==true and IsMountStale==true must be lazy-unmounted
// (see LazyUMount) before a new mfsmount can reclaim the path.
//
// Returns (false, nil) when the path is a healthy, accessible mount or
// when the path does not exist at all (nothing to recover). The error
// path is reserved for unexpected stat failures (permission denied,
// I/O error other than ENOTCONN/ESTALE, ...).
func IsMountStale(path string) (bool, error) {
	if path == "" {
		return false, errors.New("IsMountStale -- path must be provided")
	}
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	return isStaleStatError(err)
}

// isStaleStatError classifies the error returned by os.Stat on a mount
// path: ENOTCONN ("Transport endpoint is not connected") and ESTALE
// indicate a stale FUSE mount whose backing daemon is gone, while any
// other errno is surfaced to the caller. A nil error (healthy mount) is
// not stale. Extracted from IsMountStale so the classification logic is
// unit-testable without a real stale mount on the filesystem.
func isStaleStatError(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	pathErr, ok := err.(*os.PathError)
	if !ok {
		return false, err
	}
	errno, ok := pathErr.Err.(syscall.Errno)
	if !ok {
		return false, err
	}
	switch errno {
	case syscall.ENOTCONN, syscall.ESTALE:
		return true, nil
	}
	return false, err
}

func (m *Mounter) IsMounted(destPath string) (bool, error) {
	if destPath == "" {
		return false, errors.New("Mounter::IsMounted -- target must be provided")
	}

	// When routing through the host namespace, findmnt is resolved against
	// the host's filesystem (once nsenter has switched mount namespaces), so
	// checking this container's own $PATH would give a false negative.
	if !m.UseHostNamespace {
		_, err := exec.LookPath(findmntCmd)
		if err != nil {
			if err == exec.ErrNotFound {
				return false, fmt.Errorf("Mounter::IsMounted -- %q executable not found in $PATH", findmntCmd)
			}
			return false, err
		}
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", destPath, "-J"}
	out, err := m.command(findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}
		return false, fmt.Errorf("Mounter::IsMounted -- checking mounted failed: %v cmd: %q output: %q",
			err, findmntCmd, string(out))
	}

	if string(out) == "" {
		log.Warningf("Mounter::IsMounted -- %s returns no output while returning status 0 - unexpected behaviour but not an actual error", findmntCmd)
		return false, nil
	}

	var resp *findmntResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return false, fmt.Errorf("Mounter::IsMounted -- couldn't unmarshal data: %q: %s", string(out), err)
	}

	for _, fs := range resp.FileSystems {
		// check if the mount is propagated correctly. It should be set to shared, unless we run sanity tests
		if fs.Propagation != "shared" && !SanityTestRun {
			return true, fmt.Errorf("Mounter::IsMounted -- mount propagation for target %q is not enabled (%s instead of shared)", destPath, fs.Propagation)
		}
		// the mountpoint should match as well
		if fs.Target == destPath {
			return true, nil
		}
	}
	return false, nil
}
