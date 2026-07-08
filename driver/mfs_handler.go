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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	fsType         = "moosefs"
	newVolumeMode  = 0755
	getQuotaCmd    = "mfsgetquota"
	setQuotaCmd    = "mfssetquota"
	quotaLimitType = "-L"
	logsDirName    = "logs"
	volumesDirName = "volumes"
	mntDir         = "/mnt"
)

// defaultMountOptions are applied to every mfsmount invocation (both the
// shared per-node pool mount and per-volume staging mounts) to make the
// FUSE client resilient in a containerized Kubernetes environment:
//   - mfsioretries=30  : retry I/O on transient master/network errors
//     before surfacing an error to the application (default 30).
//   - mfstimeout=60    : bound how long the client blocks on an
//     unreachable master before failing I/O (default 0 = forever, which
//     hangs pods indefinitely during master outages).
//   - mfswritecachesize=256 : 256 MiB write cache (MooseFS default).
//   - allow_other      : FUSE option so application containers running
//     as a different UID/GID than the CSI plugin can access the mount
//     (required for fsGroup to work).
//
// Operators can override any of these via the "mfs_mount_options"
// configmap key (user-supplied values win on key conflict; see
// mergeMountOptions).
var defaultMountOptions = []string{
	"mfsioretries=30",
	"mfstimeout=60",
	"mfswritecachesize=256",
	"allow_other",
}

// mergeMountOptions combines base (caller-supplied) options with the
// handler's persistent mfsMountOptions and the always-on
// defaultMountOptions, de-duplicating by the "key" portion of each
// "key=value" option (bare flags like "allow_other" are treated as keys
// with an empty value). When the same key appears more than once, the
// *first* occurrence wins, with precedence: base > mfsMountOptions >
// defaults. This means callers can override defaults via either the
// volume-capability mount flags (base) or the global configmap
// (mfsMountOptions), and defaults fill in anything left unspecified.
func mergeMountOptions(base []string, mfsMountOptions string) []string {
	merged := make([]string, 0, len(base)+len(defaultMountOptions)+4)
	seen := make(map[string]struct{}, len(base)+len(defaultMountOptions)+4)

	add := func(opt string) {
		if opt == "" {
			return
		}
		key := opt
		if i := strings.IndexByte(opt, '='); i >= 0 {
			key = opt[:i]
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, opt)
	}

	for _, o := range base {
		add(o)
	}
	if mfsMountOptions != "" {
		for _, o := range strings.Split(mfsMountOptions, ",") {
			add(strings.TrimSpace(o))
		}
	}
	for _, o := range defaultMountOptions {
		add(o)
	}
	return merged
}

// todo(ad): in future possibly add more options (mount options?)
type mfsHandler struct {
	mfsmaster       string // mfsmaster address
	mfsmaster_port  int    // mfsmaster port
	rootPath        string // mfs root path
	pluginDataPath  string // plugin data path (inside rootPath)
	name            string // handler name
	hostMountPath   string // host mfs mount path
	mfsMountOptions string // mfsmount additional options
}

func NewMfsHandler(mfsmaster string, mfsmaster_port int, rootPath, pluginDataPath, name, mfsMountOptions string, num ...int) *mfsHandler {
	var numSufix = ""
	var mountOptions = ""

	if len(num) == 2 {
		if num[0] == 0 && num[1] == 1 {
			numSufix = ""
		} else {
			numSufix = fmt.Sprintf("_%02d", num[0])
		}
	} else if len(num) != 0 {
		log.Errorf("NewMfsHandler - Unexpected number of arguments: %d; expected 0 or 2", len(num))
	}

	if len(mfsMountOptions) != 0 {
		mountOptions = mfsMountOptions
	}

	return &mfsHandler{
		mfsmaster:       mfsmaster,
		mfsmaster_port:  mfsmaster_port,
		rootPath:        rootPath,
		pluginDataPath:  pluginDataPath,
		name:            name,
		hostMountPath:   path.Join(mntDir, fmt.Sprintf("%s%s", name, numSufix)),
		mfsMountOptions: mountOptions,
	}
}

func (mnt *mfsHandler) SetMfsLogging() {
	log.Infof("Setting up MooseFS Logging - path: %s", path.Join(mnt.rootPath, mnt.pluginDataPath, logsDirName))
	mfsLogFile := &lumberjack.Logger{
		Filename:   path.Join(mnt.HostPathToLogs(), fmt.Sprintf("%s.log", mnt.name)),
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     0,
		Compress:   true,
	}
	mw := io.MultiWriter(os.Stderr, mfsLogFile)
	log.SetOutput(mw)
	log.Info("MooseFS Logging set up!")
}

func (mnt *mfsHandler) VolumeExist(volumeId string) (bool, error) {
	path := mnt.HostPathToVolume(volumeId)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (mnt *mfsHandler) MountVolumeExist(volumeId string) (bool, error) {
	path := mnt.HostPathToMountVolume(volumeId)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (mnt *mfsHandler) CreateMountVolume(volumeId string) error {
	path := mnt.HostPathToMountVolume(volumeId)
	if err := os.MkdirAll(path, newVolumeMode); err != nil {
		return err
	}
	return nil
}

func (mnt *mfsHandler) CreateVolume(volumeId string, size int64) (int64, error) {
	path := mnt.HostPathToVolume(volumeId)
	if err := os.MkdirAll(path, newVolumeMode); err != nil {
		return 0, err
	}
	if size == 0 {
		return 0, nil
	}
	acquiredSize, err := mnt.SetQuota(volumeId, size)
	if err != nil {
		return 0, err
	}
	return acquiredSize, nil
}

func (mnt *mfsHandler) DeleteVolume(volumeId string) error {
	path := mnt.HostPathToVolume(volumeId)
	if err := os.RemoveAll(path); err != nil {
		// todo(ad): fix msg
		log.Errorf("-------------------ControllerService::DeleteVolume -- Couldn't remove volume %s in directory %s. Error: %s",
			volumeId, path, err.Error())
		return err
	}

	return nil
}

func (mnt *mfsHandler) GetQuota(volumeId string) (int64, error) {
	log.Infof("GetQuota - volumeId: %s", volumeId)

	//path := mnt.MfsPathToVolume(volumeId)
	path := mnt.HostPathToVolume(volumeId)

	cmd := exec.Command(getQuotaCmd, path)
	//cmd.Dir = mnt.hostMountPath
	out, err := cmd.CombinedOutput()

	if err != nil {
		return 0, fmt.Errorf("GetQuota: Error while executing command %s %s. Error: %s output: %v", getQuotaCmd, path, err.Error(), string(out))
	}
	if quotaLimit, err := parseMfsQuotaToolsOutput(string(out)); err != nil {
		return 0, err
	} else if quotaLimit == -1 {
		return 0, fmt.Errorf("GetQuota: Quota for volume %s is not set or %s output is incorrect. Output: %s", volumeId, getQuotaCmd, string(out))
	} else {
		return quotaLimit, nil
	}
}

func (mnt *mfsHandler) SetQuota(volumeId string, size int64) (int64, error) {
	log.Infof("SetQuota - volumeId: %s, size: %d", volumeId, size)

	//path := mnt.MfsPathToVolume(volumeId)
	path := mnt.HostPathToVolume(volumeId)
	if size <= 0 {
		return 0, errors.New("SetQuota: size must be positive")
	}
	setQuotaArgs := []string{quotaLimitType, strconv.FormatInt(size, 10), path}
	cmd := exec.Command(setQuotaCmd, setQuotaArgs...)
	//cmd.Dir = mnt.hostMountPath
	out, err := cmd.CombinedOutput()

	if err != nil {
		return 0, fmt.Errorf("SetQuota: Error while executing command %s %v. Error: %s output: %v", setQuotaCmd, setQuotaArgs, err.Error(), string(out))
	}
	if quotaLimit, err := parseMfsQuotaToolsOutput(string(out)); err != nil {
		return 0, err
	} else if quotaLimit == -1 {
		return 0, fmt.Errorf("SetQuota: Quota for volume %s is not set or %s output is incorrect. Output: %s", volumeId, setQuotaCmd, string(out))
	} else {
		return quotaLimit, nil
	}
}

func parseMfsQuotaToolsOutput(output string) (int64, error) {
	var cols []string
	var s string

	lines := strings.Split(output, "\n")
	ll := len(lines)

	switch ll {
	case 8:
		// mfssetquota new format output
		cols = strings.Split(lines[ll-4], "|")
		s = strings.TrimSpace(cols[4])
	case 6:
		// mfsgetquota old format output
		cols := strings.Split(lines[ll-4], "|")
		s = strings.TrimSpace(cols[3])
	case 0:
		return -1, errors.New("error while parsing mfsgetquota tool output (empty output)")
	default:
		return -1, fmt.Errorf("error while parsing mfsgetquota tool output (unexpected number of lines); output: %s", output)
	}

	if s == "-" {
		// no quota set
		return -1, nil
	}

	quotaLimit, err := strconv.ParseInt(s, 10, 64)

	if err != nil {
		return -1, err
	}

	return quotaLimit, nil
}

// mfsSourceURI builds the "master:port:remotePath" source string accepted by
// mfsmount for the given MooseFS sub-path (relative to this handler's
// rootPath). Pulled out as its own function so the (deterministic) source
// construction is unit-testable independently of actually invoking mount(8).
func (mnt *mfsHandler) mfsSourceURI(subPath string) string {
	remotePath := mnt.rootPath
	if subPath != "" {
		remotePath = path.Join(mnt.rootPath, subPath)
	}
	return fmt.Sprintf("%s:%d:%s", mnt.mfsmaster, mnt.mfsmaster_port, remotePath)
}

// Mount mounts mfsclient at speciefied earlier point
func (mnt *mfsHandler) MountMfs() error {
	mounter := Mounter{}
	mountSource := mnt.mfsSourceURI("")

	mountOptions := mergeMountOptions(nil, mnt.mfsMountOptions)

	log.Infof("MountMfs - source: %s, target: %s, options: %v", mountSource, mnt.hostMountPath, mountOptions)

	if isMounted, err := mounter.IsMounted(mnt.hostMountPath); err != nil {
		return err
	} else if isMounted {
		if stale, _ := IsMountStale(mnt.hostMountPath); stale {
			log.Warnf("MountMfs - mount at %s is stale (ENOTCONN/ESTALE), lazy-unmounting before remount", mnt.hostMountPath)
			if err := mounter.LazyUMount(mnt.hostMountPath); err != nil {
				return fmt.Errorf("MountMfs - lazy unmount of stale %s failed: %w", mnt.hostMountPath, err)
			}
		} else {
			log.Infof("MountMfs - mount at %s is healthy, reusing", mnt.hostMountPath)
			return nil
		}
	}
	if err := os.RemoveAll(mnt.hostMountPath); err != nil {
		return err
	}
	if err := mounter.Mount(mountSource, mnt.hostMountPath, fsType, mountOptions...); err != nil {
		return err
	}
	log.Infof("MountMfs - Successfully mounted %s to %s", mountSource, mnt.hostMountPath)
	return nil
}

func (mnt *mfsHandler) BindMount(mfsSource string, target string, options ...string) error {
	return mnt.BindMountWithFSGroup(mfsSource, target, nil, options...)
}

// MountMfsVolumeStaged mounts the given MooseFS-relative sub-path (as
// returned by MfsPathToVolume) *directly* onto stagingTarget -- i.e. it is a
// real, independent mfsmount FUSE mount, not a bind-mount derived from the
// shared per-node pool mount used by MountMfs/BindMountWithFSGroup.
//
// This is used by NodeStageVolume. Unlike the pool mount, it is executed via
// the host-namespace-aware Mounter (when HostNamespaceMount is enabled), so
// the resulting mfsmount daemon is reparented to the host and is not killed
// when the csi-moosefs-node container restarts. stagingTarget must live
// under a path that is already bind-mounted into this container with
// Bidirectional/shared propagation (e.g. under /var/lib/kubelet, as provided
// by kubelet), so that directory creation and mount visibility are
// consistent between the container and host mount namespaces.
//
// See: https://github.com/moosefs/moosefs-csi/issues/32
func (mnt *mfsHandler) MountMfsVolumeStaged(mfsSubPath string, stagingTarget string, options ...string) error {
	mounter := Mounter{UseHostNamespace: HostNamespaceMount}
	mountSource := mnt.mfsSourceURI(mfsSubPath)

	// Merge the volume-capability mount flags (options) with the handler's
	// persistent mfsMountOptions (which typically carry authentication
	// such as mfsmd5pass=...) and the always-on resilient defaults.
	// Without this, staging mounts fail to authenticate against a master
	// that requires a password. See issue #32.
	mountOptions := mergeMountOptions(options, mnt.mfsMountOptions)

	log.Infof("MountMfsVolumeStaged - source: %s, target: %s, options: %v, hostNamespace: %v",
		mountSource, stagingTarget, mountOptions, mounter.UseHostNamespace)

	if isMounted, err := mounter.IsMounted(stagingTarget); err != nil {
		return err
	} else if isMounted {
		if stale, _ := IsMountStale(stagingTarget); stale {
			log.Warnf("MountMfsVolumeStaged - staging target %s is stale (ENOTCONN/ESTALE), lazy-unmounting before remount", stagingTarget)
			if err := mounter.LazyUMount(stagingTarget); err != nil {
				return fmt.Errorf("MountMfsVolumeStaged - lazy unmount of stale %s failed: %w", stagingTarget, err)
			}
		} else {
			log.Infof("MountMfsVolumeStaged - target %s is already mounted and healthy, reusing", stagingTarget)
			return nil
		}
	}
	if err := mounter.MountDetaching(mountSource, stagingTarget, fsType, mountOptions...); err != nil {
		return err
	}
	// setsid --fork reparents the mfsmount daemon to host init and gives it
	// its own session/process group, but it does NOT move it out of this
	// container's cgroup. On containerd, stopping the container kills every
	// process in the container's cgroup scope -- including the nsenter-spawned
	// mfsmount -- which is exactly the ENOTCONN-on-restart problem issue #32
	// is about. Move the staging mfsmount daemon(s) into the host root cgroup
	// so their lifetime is truly decoupled from the csi-moosefs-node pod.
	if HostNamespaceMount {
		if err := reparentMfsmountToHostCgroup(stagingTarget); err != nil {
			// Non-fatal: the mount itself succeeded. A failure here means
			// the daemon may be killed on container stop, degrading to the
			// pre-fix behavior (stale mount recovered on next NodeStage).
			log.Warnf("MountMfsVolumeStaged - could not move mfsmount for %s to host cgroup: %v (volume will work but may not survive node-plugin restart)", stagingTarget, err)
		}
	}
	log.Infof("MountMfsVolumeStaged - Successfully mounted %s to %s", mountSource, stagingTarget)
	return nil
}

// UnmountStaged unmounts a staging target previously created by
// MountMfsVolumeStaged.
func (mnt *mfsHandler) UnmountStaged(stagingTarget string) error {
	mounter := Mounter{UseHostNamespace: HostNamespaceMount}
	log.Infof("UnmountStaged - target: %s, hostNamespace: %v", stagingTarget, mounter.UseHostNamespace)
	if mounted, err := mounter.IsMounted(stagingTarget); err != nil {
		return err
	} else if mounted {
		if err := mounter.UMount(stagingTarget); err != nil {
			// Fall back to a lazy unmount for stale FUSE mounts that a
			// regular umount cannot detach (ENOTCONN / daemon gone).
			log.Warnf("UnmountStaged - regular umount of %s failed (%v), attempting lazy unmount", stagingTarget, err)
			return mounter.LazyUMount(stagingTarget)
		}
	}
	log.Infof("UnmountStaged - target %s was already unmounted", stagingTarget)
	return nil
}

// UnmountPool unmounts this handler's shared per-node pool mount
// (/mnt/<nodeId>). It is called from NodeService.Stop() during graceful
// shutdown so that the MooseFS master sees a clean session close rather
// than a dropped connection. It falls back to a lazy unmount when the
// mount is stale. Per-volume staging mounts are deliberately not
// touched here (see Stopper docs in service.go).
func (mnt *mfsHandler) UnmountPool() error {
	mounter := Mounter{}
	log.Infof("UnmountPool - target: %s", mnt.hostMountPath)
	if mounted, err := mounter.IsMounted(mnt.hostMountPath); err != nil {
		return err
	} else if mounted {
		if err := mounter.UMount(mnt.hostMountPath); err != nil {
			log.Warnf("UnmountPool - regular umount of %s failed (%v), attempting lazy unmount", mnt.hostMountPath, err)
			return mounter.LazyUMount(mnt.hostMountPath)
		}
	}
	log.Infof("UnmountPool - target %s was already unmounted", mnt.hostMountPath)
	return nil
}

// BindMountAbs bind-mounts an already-resolved absolute source path (e.g. a
// staging target created by MountMfsVolumeStaged) onto target. Unlike
// BindMountWithFSGroup, source is used as-is and is NOT resolved relative to
// this handler's pooled root mount, and it always runs in the container's
// own namespace: since source is itself a durable, host-independent mount
// (when created via MountMfsVolumeStaged), and target lives under the
// Bidirectional-shared kubelet hierarchy, this plain bind-mount is
// automatically visible to, and independent of, the host.
func (mnt *mfsHandler) BindMountAbs(source, target string, options ...string) error {
	mounter := Mounter{}
	log.Infof("BindMountAbs - source: %s, target: %s, options: %v", source, target, options)
	if isMounted, err := mounter.IsMounted(target); err != nil {
		return err
	} else if !isMounted {
		return mounter.Mount(source, target, fsType, append(options, "bind")...)
	}
	log.Infof("BindMountAbs - target %s is already mounted", target)
	return nil
}

func (mnt *mfsHandler) BindMountWithFSGroup(mfsSource string, target string, fsGroup *int64, options ...string) error {
	mounter := Mounter{}
	source := mnt.HostPathTo(mfsSource)
	log.Infof("BindMountWithFSGroup - source: %s, target: %s, fsGroup: %v, options: %v", source, target, fsGroup, options)
	
	if isMounted, err := mounter.IsMounted(target); err != nil {
		return err
	} else if !isMounted {
		if err := mounter.Mount(source, target, fsType, append(options, "bind")...); err != nil {
			return err
		}
		
		// Apply fsGroup permissions if specified
		if fsGroup != nil {
			if err := mnt.applyFSGroupPermissions(source, *fsGroup); err != nil {
				log.Errorf("BindMountWithFSGroup - Failed to apply fsGroup permissions: %v", err)
				// Don't fail the mount for permission errors, log and continue
			}
		}
	} else {
		log.Infof("BindMountWithFSGroup - target %s is already mounted", target)
	}
	return nil
}

func (mnt *mfsHandler) BindUMount(target string) error {
	mounter := Mounter{}
	log.Infof("BindUMount - target: %s", target)
	if mounted, err := mounter.IsMounted(target); err != nil {
		return err
	} else if mounted {
		if err := mounter.UMount(target); err != nil {
			return err
		}
	} else {
		log.Infof("BindUMount - target %s was already unmounted", target)
	}
	return nil
}

// HostPathToVolume returns absoluthe path to given volumeId on host mfsclient mountpoint
func (mnt *mfsHandler) HostPathToVolume(volumeId string) string {
	return path.Join(mnt.hostMountPath, mnt.pluginDataPath, volumesDirName, volumeId)
}

func (mnt *mfsHandler) MfsPathToVolume(volumeId string) string {
	return path.Join(mnt.pluginDataPath, volumesDirName, volumeId)
}

func (mnt *mfsHandler) HostPathToMountVolume(volumeId string) string {
	return path.Join(mnt.hostMountPath, mnt.pluginDataPath, "mount_volumes", volumeId)
}

func (mnt *mfsHandler) HostPathToLogs() string {
	return path.Join(mnt.hostMountPath, mnt.pluginDataPath, logsDirName)
}

func (mnt *mfsHandler) HostPluginDataPath() string {
	return path.Join(mnt.hostMountPath, mnt.pluginDataPath)
}

func (mnt *mfsHandler) HostPathTo(to string) string {
	return path.Join(mnt.hostMountPath, to)
}

// applyFSGroupPermissions applies the specified fsGroup ownership to the volume directory
// This function sets the group ownership of the volume root directory to the fsGroup
// and ensures it's group-writable (0775 permissions)
func (mnt *mfsHandler) applyFSGroupPermissions(volumePath string, fsGroup int64) error {
	log.Infof("applyFSGroupPermissions - path: %s, fsGroup: %d", volumePath, fsGroup)
	
	// Get current file info
	fileInfo, err := os.Stat(volumePath)
	if err != nil {
		return fmt.Errorf("failed to stat volume path %s: %v", volumePath, err)
	}
	
	// Get current uid (should remain unchanged)
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get syscall.Stat_t for %s", volumePath)
	}
	currentUID := int(stat.Uid)
	
	// Change ownership to current uid and specified fsGroup
	if err := os.Chown(volumePath, currentUID, int(fsGroup)); err != nil {
		return fmt.Errorf("failed to chown %s to %d:%d: %v", volumePath, currentUID, fsGroup, err)
	}
	
	// Set permissions to 0775 (owner+group writable, others readable)
	// This ensures the fsGroup can write to the volume
	if err := os.Chmod(volumePath, 0775); err != nil {
		return fmt.Errorf("failed to chmod %s to 0775: %v", volumePath, err)
	}
	
	log.Infof("applyFSGroupPermissions - successfully applied fsGroup %d to %s", fsGroup, volumePath)
	return nil
}

// reparentMfsmountToHostCgroup moves any mfsmount daemon mounted at target
// out of the csi-moosefs-node container's cgroup and into the host root
// cgroup, so the daemon is not killed when the container stops.
//
// Background: nsenter --target 1 --mount --pid runs the mount in the host's
// mount and PID namespaces, and setsid --fork gives the daemon its own
// session, but neither changes the process's *cgroup*. On containerd/K8s the
// container's cgroup scope (cri-containerd-<id>.scope) is destroyed on
// container stop and every process still in it receives SIGKILL -- including
// the FUSE daemon we spawned via nsenter. The result is a stale FUSE mount
// that returns ENOTCONN to every I/O until the app pod is restaged. This is
// the crux of issue #32.
//
// The fix is to write the daemon's PID into the host root cgroup's
// cgroup.procs file (unified cgroup-v2: /sys/fs/cgroup/cgroup.procs; legacy
// cgroup-v1: /sys/fs/cgroup/tasks for the root). After this the daemon is
// accounted to the host, not the container, and survives container stop.
//
// target is the mount point (e.g. the staging globalmount path). We find the
// daemon PID(s) by scanning /proc/*/mountinfo for processes whose root
// points at a FUSE mount of target. Cheaper and more reliable than pgrep:
// works even if mfsmount renames itself.
func reparentMfsmountToHostCgroup(target string) error {
	pids, err := findFuseDaemonPIDs(target)
	if err != nil {
		return fmt.Errorf("find mfsmount pids for %s: %w", target, err)
	}
	if len(pids) == 0 {
		return fmt.Errorf("no mfsmount daemon found for %s", target)
	}

	// cgroup-v2 unified hierarchy is the standard on modern distros
	// (Ubuntu 24.04, Debian 12, RHEL 9, ...). cgroup-v1 fallback covers
	// older systems. We try v2 first, then v1.
	hostCgroupProcs := "/sys/fs/cgroup/cgroup.procs"       // v2 root
	hostCgroupTasks := "/sys/fs/cgroup/tasks"              // v1 root
	moved := 0
	var lastErr error
	for _, pid := range pids {
		pidStr := strconv.Itoa(pid)
		if err := writeCgroupProc(hostCgroupProcs, pidStr); err == nil {
			log.Infof("reparentMfsmountToHostCgroup - moved PID %d to %s", pid, hostCgroupProcs)
			moved++
			continue
		} else {
			lastErr = err
		}
		if err := writeCgroupProc(hostCgroupTasks, pidStr); err == nil {
			log.Infof("reparentMfsmountToHostCgroup - moved PID %d to %s (v1)", pid, hostCgroupTasks)
			moved++
			continue
		} else {
			lastErr = err
		}
		log.Warnf("reparentMfsmountToHostCgroup - could not move PID %d: %v", pid, err)
	}
	if moved == 0 {
		return fmt.Errorf("could not move any mfsmount PID to host cgroup: %w", lastErr)
	}
	return nil
}

// writeCgroupProc writes a PID string to a cgroup.procs/tasks file. The
// write requires the process to have write permission on the target cgroup
// (the csi-moosefs-node pod runs privileged, so this is satisfied).
func writeCgroupProc(path, pidStr string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pidStr)
	return err
}

// findFuseDaemonPIDs returns the PIDs of mfsmount processes whose command
// line references the given target mount point. mfsmount renames itself to
// "mfsmount (mounted on: <target>)", so matching /proc/<pid>/cmdline is
// reliable and avoids parsing mountinfo across PID namespaces. It also
// avoids the race where mountinfo is not yet populated right after spawn.
//
// /proc is visible with host PIDs because the csi-moosefs-node pod runs
// with hostPID: true.
func findFuseDaemonPIDs(target string) ([]int, error) {
	// The daemon may still be starting up right after setsid --fork returns.
	// Retry briefly so we don't miss it and silently fall back to the old
	// (kill-on-container-stop) behavior.
	var pids []int
	for attempt := 0; attempt < 5; attempt++ {
		pids = scanProcForMfsmountTarget(target)
		if len(pids) > 0 {
			return pids, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, nil
}

// scanProcForMfsmountTarget does a single pass over /proc looking for
// mfsmount processes whose cmdline contains target.
func scanProcForMfsmountTarget(target string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		// cmdline is NUL-separated; mfsmount sets its title to
		// "mfsmount (mounted on: <target>)" so the target path appears in
		// the first (and only) argv element.
		cmd := strings.ReplaceAll(string(cmdline), "\x00", " ")
		if !strings.Contains(cmd, "mfsmount") {
			continue
		}
		if !strings.Contains(cmd, target) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}
