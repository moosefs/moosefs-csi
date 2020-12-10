package driver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// assumes lock
func (cs *ControllerService) _loadVolumes() (map[string]*mfsVolume, error) {
	volumeData, err := ioutil.ReadFile(cs.volumesDataPath)
	if err != nil {
		return nil, err
	}
	var volumes map[string]*mfsVolume
	err = json.Unmarshal(volumeData, &volumes)
	if err != nil {
		return nil, err
	}
	if volumes == nil {
		volumes = make(map[string]*mfsVolume)
	}
	return volumes, nil
}

// assumes lock
func (cs *ControllerService) _saveVolume(volumes map[string]*mfsVolume) error {
	d, err := json.MarshalIndent(volumes, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(cs.volumesDataPath, d, 0666)
	if err != nil {
		return err
	}
	return nil
}

// asssumes lock
func (cs *ControllerService) _getVolume(volumeId string) (*mfsVolume, error) {
	vols, err := cs._loadVolumes()
	if err != nil {
		return nil, err
	}
	res, ok := vols[volumeId]
	if !ok {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume %s not found", volumeId))
	}
	return res, nil
}

// assumes lock
func (cs *ControllerService) _putVolume(volume *mfsVolume) error {
	vols, err := cs._loadVolumes()
	if err != nil {
		return err
	}
	_, ok := vols[volume.VolumeId]
	if ok {
		return status.Error(codes.AlreadyExists, fmt.Sprintf("Volume %s already exists", volume.VolumeId))
	}
	vols[volume.VolumeId] = volume
	d, err := json.MarshalIndent(vols, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(cs.volumesDataPath, d, 0666)
	if err != nil {
		return err
	}
	return nil
}

////

type mfsVolume struct {
	VolumeId       string `json:"volume_id"`
	VolumeCapacity uint64 `json:"volume_capacity"`
	VolumeDir      string `json:"volume_dir"`
	SubMount       bool   `json:"sub_mount"`
}

func (cs *ControllerService) findVolume(volumeId string) (bool, error) {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	_, err := cs._getVolume(volumeId)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
