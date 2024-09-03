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

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	fsType          = "moosefs"
	newVolumeMode   = 0755
	getQuotaCmd     = "mfsgetquota"
	setQuotaCmd     = "mfssetquota"
	quotaLimitType  = "-L"
	quotaLimitRow   = 2
	quotaLimitCol   = 3
	logsDirName     = "logs"
	volumesDirName  = "volumes"
	mvolumesDirName = "mount_volumes"
	mntDir          = "/mnt"
)

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
	log.Infof("Setting up Mfs Logging. Mfs path: %s", path.Join(mnt.rootPath, mnt.pluginDataPath, logsDirName))
	mfsLogFile := &lumberjack.Logger{
		Filename:   path.Join(mnt.HostPathToLogs(), fmt.Sprintf("%s.log", mnt.name)),
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     0,
		Compress:   true,
	}
	mw := io.MultiWriter(os.Stderr, mfsLogFile)
	log.SetOutput(mw)
	log.Info("Mfs Logging set up!")
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

	if ll == 8 {
		// new mfsgetquota output format
		cols = strings.Split(lines[ll-4], "|")
		s = strings.TrimSpace(cols[4])
	} else if ll == 6 {
		// old mfsgetquota output format
		cols := strings.Split(lines[ll-4], "|")
		s = strings.TrimSpace(cols[3])
	} else {
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

// Mount mounts mfsclient at speciefied earlier point
func (mnt *mfsHandler) MountMfs() error {
	var mountOptions []string
	mounter := Mounter{}
	mountSource := fmt.Sprintf("%s:%d:%s", mnt.mfsmaster, mnt.mfsmaster_port, mnt.rootPath)

	if len(mnt.mfsMountOptions) != 0 {
		mountOptions = strings.Split(mnt.mfsMountOptions, ",")
	} else {
		mountOptions = make([]string, 0)
	}

	log.Infof("MountMfs - source: %s, target: %s, options: %v", mountSource, mnt.hostMountPath, mountOptions)

	if isMounted, err := mounter.IsMounted(mnt.hostMountPath); err != nil {
		return err
	} else if isMounted {
		log.Warnf("MountMfs - Mount found in %s. Unmounting...", mnt.hostMountPath)
		if err = mounter.UMount(mnt.hostMountPath); err != nil {
			return err
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
	mounter := Mounter{}
	source := mnt.HostPathTo(mfsSource)
	log.Infof("BindMount - source: %s, target: %s, options: %v", source, target, options)
	if isMounted, err := mounter.IsMounted(target); err != nil {
		return err
	} else if !isMounted {
		if err := mounter.Mount(source, target, fsType, append(options, "bind")...); err != nil {
			return err
		}
	} else {
		log.Infof("BindMount - target %s is already mounted", target)
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
