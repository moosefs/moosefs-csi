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
}

var _ MounterInterface = &Mounter{}

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

	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(destPath, newDirMode)
	if err != nil {
		return err
	}
	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::Mount -- mounting failed: %v cmd: '%s %s' output: %q",
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

	out, err := exec.Command(umountCmd, umountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Mounter::UMount -- mounting failed: %v cmd: '%s %s' output: %q",
			err, umountCmd, strings.Join(umountArgs, " "), string(out))
	}

	return nil
}

func (m *Mounter) IsMounted(destPath string) (bool, error) {
	if destPath == "" {
		return false, errors.New("Mounter::IsMounted -- target must be provided")
	}

	_, err := exec.LookPath(findmntCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("Mounter::IsMounted -- %q executable not found in $PATH", findmntCmd)
		}
		return false, err
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", destPath, "-J"}
	out, err := exec.Command(findmntCmd, findmntArgs...).CombinedOutput()
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
