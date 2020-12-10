package driver

import (
	"os"
	log "github.com/sirupsen/logrus"
	"path"
)

const (
	fsType = "moosefs"
)


type MfsMountPoint struct {
	mfsmaster    string
	rootDir      string
	pluginDir    string // plugin data dir
	hostPath     string
}

func NewMfsMountPoint(mfsmaster, rootDir, hostPath, pluginDir string) (*MfsMountPoint, error) {
	return &MfsMountPoint{
		mfsmaster: mfsmaster,
		rootDir:   rootDir,
		pluginDir: pluginDir,
		hostPath:  hostPath,
	}, nil
}

func (mnt *MfsMountPoint) Mount() error {
	mounter := Mounter{}
	mountSource := mnt.mfsmaster + ":" + mnt.rootDir
	mountOptions := make([]string, 0)

	log.Infof("===== TUTEJ MOUNTUJEMY ", mountSource, mountOptions)

	isMounted, err := mounter.IsMounted(mnt.hostPath)
	if err != nil {
		return err
	}
	if !isMounted {
		log.Infof("MfsMountPoint::Mount -- Nothing mount on %s", mnt.hostPath)
	} else {
		log.Infof("MfsMountPoint::Mount -- Unmounting from %s", mnt.hostPath)
		if err = mounter.UMount(mnt.hostPath); err != nil {
			return err
		}
	}
	if err = os.RemoveAll(mnt.hostPath); err != nil {
		return err
	}

	if err := mounter.Mount(mountSource, mnt.hostPath, fsType, mountOptions...); err != nil {
//		log.Errorf("MfsMountPoint::Mount -- Error while mounting %s to %s (%s)", mountSource, mnt.hostPath, err.Error())
		return err
	}
	log.Infof("MfsMountPoint::Mount -- Successfully mounted %s to %s", mountSource, mnt.hostPath)

	return nil
}

func (mnt *MfsMountPoint) HostPluginPath() string {
	return path.Join(mnt.hostPath, mnt.pluginDir)
}

func (mnt *MfsMountPoint) MfsPluginPath() string {
	return mnt.pluginDir
}

func (mnt *MfsMountPoint) HostPathTo(to string) string {
	return path.Join(mnt.hostPath, to)
}
