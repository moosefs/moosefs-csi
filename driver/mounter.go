/*
 *
 *
 *
 *
 *
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

type Mounter interface {
	// Mount a volume
	Mount(sourcePath string, destPath, mountType string, opts ...string) error

	// Unmount a volume
	UMount(destPath string) error

	// Verify mount
	IsMounted(sourcePath string, destPath string) (bool, error)
}

type mounter struct {
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

/*
 * Mounts the mooseFs filesystem
 *
 *
 */

func (m *mounter) Mount(sourcePath, destPath, mountType string, opts ...string) error {
	mountCmd := "mount"
	mountArgs := []string{}

	if sourcePath == "" {
		return errors.New("source is not specified for mounting the volume")
	}

	if destPath == "" {
		return errors.New("Destination path is not specified for mounting the volume")
	}

	mountArgs = append(mountArgs, "-t", mountType)
	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}

	mountArgs = append(mountArgs, sourcePath)
	mountArgs = append(mountArgs, destPath)

	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(destPath, 0750)
	if err != nil {
		return err
	}

	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}

	return nil
}

/*
 * Un-Mount the moooseFs filesystem
 *
 *
 *
 *
 */

func (m *mounter) UMount(destPath string) error {
	umountCmd := "umount"
	umountArgs := []string{}

	if destPath == "" {
		return errors.New("Destination path not specified for unmounting volume")
	}

	umountArgs = append(umountArgs, destPath)

	out, err := exec.Command(umountCmd, umountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %v cmd: '%s %s' output: %q",
			err, umountCmd, strings.Join(umountArgs, " "), string(out))
	}

	return nil
}

/*
 *	Checks if the given src and dst path are mounted
 *
 *
 *	courtesy: https://github.com/digitalocean/csi-digitalocean/blob/master/driver/mounter.go
 */

func (m *mounter) IsMounted(sourcePath, destPath string) (bool, error) {
	if sourcePath == "" {
		return false, errors.New("source is not specified for checking the mount")
	}

	if destPath == "" {
		return false, errors.New("target is not specified for checking the mount")
	}

	findmntCmd := "findmnt"
	_, err := exec.LookPath(findmntCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable not found in $PATH", findmntCmd)
		}
		return false, err
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", sourcePath, "-J"}
	out, err := exec.Command(findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}

		return false, fmt.Errorf("checking mounted failed: %v cmd: %q output: %q",
			err, findmntCmd, string(out))
	}

	var resp *findmntResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return false, fmt.Errorf("couldn't unmarshal data: %q: %s", string(out), err)
	}

	targetFound := false
	for _, fs := range resp.FileSystems {
		// check if the mount is propagated correctly. It should be set to shared.
		if fs.Propagation != "shared" {
			return true, fmt.Errorf("mount propagation for target %q is not enabled or the block device %q does not exist anymore", destPath, sourcePath)
		}

		// the mountpoint should match as well
		if fs.Target == destPath {
			targetFound = true
		}
	}

	return targetFound, nil
}
