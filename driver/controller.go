/*
   Copyright 2019 Tuxera Oy. All Rights Reserved.

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
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	_ = iota
	// KB ...std sizes
	KB = 1 << (10 * iota)
	MB
	GB
	TB
)

const (
	defaultVolumeSizeInGB = 16
)

var (
	supportedAccessMode = []*csi.VolumeCapability_AccessMode{
		{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	supportedAccessModeMode = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	}
	moosefsVolumeCapability = &csi.VolumeCapability_MountVolume{
		FsType: "moosefs",
	}
)

type ControllerService struct {
	csi.ControllerServer
	*IdentityService
	*PluginService

	ctlMount        *MfsMountPoint
	lock            sync.Mutex
	volumesDataPath string

	mfslog *MfsLog
}

func (cs *ControllerService) Register(srv *grpc.Server) {
	log.Infof("ControllerService::Register")
	cs.IdentityService.Register(srv)
	csi.RegisterControllerServer(srv, cs)
}

func NewControllerService(mfsmaster, rootDir, pluginDataDir string) (*ControllerService, error) {
	cs := &ControllerService{}
	cs.ctlMount, _ = NewMfsMountPoint(mfsmaster, rootDir, fmt.Sprintf("/mnt/mount_controller"), pluginDataDir)
	if err := cs.ctlMount.Mount(); err != nil {
		return nil, err
	}
	cs.volumesDataPath = path.Join(cs.ctlMount.HostPluginPath(), "volumes.json")

	cs.mfslog = &MfsLog{
		logfile: path.Join(fmt.Sprintf("/mnt/mount_controller"),
			pluginDataDir, fmt.Sprintf("logz_controller")),
		active: false,
	}

	return cs, nil
}

// CreateVolume creates a new volume from the given request. The function is idempotent.
func (cs *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		log.Errorf("CreateVolume: Name must be provided")
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: Name must be provided")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		log.Errorf("CreateVolume: Volume capabilities must be provided")
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: Volume capabilities must be provided")
	}
	size, err := getRequestCapacity(req.CapacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			log.Errorf("CreateVolume: Block storage not supported")
			return nil, status.Error(codes.InvalidArgument, "CreateVolume: Block storage not supported")
		}
	}
	//	log.Infof("-------------------- volumeCapabilities(1): |%v|", req.GetVolumeCapabilities())
	//	log.Infof("-------------------- volumeCapabilities(2): |%+v|", req.GetVolumeCapabilities())
	//	log.Infof("+++++++++++++++ %v", req.GetVolumeCapabilities()[0].GetAccessType())

	if req.VolumeContentSource != nil {
		log.Infof("ControllerService::CreateVolume ---- content source non empty %s", req.VolumeContentSource.GetType())
		log.Infof("ControllerService::CreateVolume ---- cd %s", req.VolumeContentSource.GetVolume().VolumeId)
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: VolumeContentSource not supported")
	}

	log.Infof("ControllerService::CreateVolume (name: %s, size: %d)", req.Name, size)

	volumeContextUpdate, err := cs.newVolume(req.Name, uint64(size), req.GetParameters())
	if err != nil {
		return nil, status.Error(codes.AlreadyExists, err.Error())
	}

	volumeContext := req.GetParameters()
	if volumeContext == nil {
		volumeContext = make(map[string]string)
	}
	for k, v := range volumeContextUpdate {
		volumeContext[k] = v
	}

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      req.GetName(),
			CapacityBytes: size,
			VolumeContext: volumeContext,
		},
	}
	return resp, nil
}

func (cs *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		log.Errorf("DeleteVolume: VolumeId must be provided")
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume: VolumeId must be provided")
	}
	log.Infof("ControllerService::DeleteVolume (volume_id: %s)", req.VolumeId)

	if err := cs.delVolume(req.VolumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if req.VolumeId == "" {
		log.Errorf("ControllerExpandVolume: VolumeId must be provided")
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume: VolumeId must be provided")
	}
	size, err := getRequestCapacity(req.CapacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	log.Infof("ControllerService::ControllerExpandVolume (volume_id: %s, size: %d)", req.VolumeId, size)

	acquiredSize, err := cs.expandVolume(req.VolumeId, uint64(size))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         int64(acquiredSize),
		NodeExpansionRequired: false,
	}, nil
}

func (cs *ControllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		log.Errorf("ValidateVolumeCapabilities: VolumeId must be provided")
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities: VolumeId must be provided")
	}
	if len(req.VolumeCapabilities) == 0 || req.VolumeCapabilities == nil {
		log.Errorf("ValidateVolumeCapabilities: VolumeCapabilities must be provided")
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities: VolumeCapabilities must be provided")
	}

	volumeDir := path.Join(cs.ctlMount.MfsPluginPath(), req.VolumeId)
	exists, err := pathExists(path.Join(cs.ctlMount.hostPath, volumeDir))
	if err != nil {
		return nil, err
	}
	if !exists {
		log.Errorf("ValidateVolumeCapabilities: Volume not found")
		return nil, status.Error(codes.NotFound, "ValidateVolumeCapabilities: Volume not found")
	}

	log.Infof("ControllerService::ValidateVolumeCapabilities (volume_id: %s)", req.VolumeId)

	/*
		if found, err := cs.findVolume(req.VolumeId); found == false {
			log.Infof("ControllerService::ValidateVolumeCapabilities -- Coudn't find volume %s.", req.VolumeId)
			return nil, status.Error(codes.NotFound, err.Error())
		}
	*/

	resp := &csi.ValidateVolumeCapabilitiesResponse{}
	//	resp := &csi.ValidateVolumeCapabilitiesResponse{
	//		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
	//			VolumeCapabilities: volCap,
	//		},
	//	}

	ok := true
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			ok = false
		}
	}
	if ok {
		resp.Confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: req.GetVolumeCapabilities()}
	}
	return resp, nil
}

func (cs *ControllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	log.Infof("ControllerService::ControllerGetCapabilities")

	var caps []*csi.ControllerServiceCapability
	for _, capa := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		//	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	} {
		caps = append(caps, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capa,
				},
			},
		})
	}

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (cs *ControllerService) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log.Infof("ControllerService::ListVolumes")
	return nil, status.Errorf(codes.Unimplemented, "method ListVolumes not implemented")
}

func (cs *ControllerService) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateSnapshot not implemented")
}

func (cs *ControllerService) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteSnapshot not implemented")
}

func (cs *ControllerService) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSnapshots not implemented")
}

func (cs *ControllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Info("ControllerService::ControllerPublishVolume")
	cs.mfslog.Log(fmt.Sprintf("ControllerService::ControllerPublishVolume (volume_id %s)", req.VolumeId))

	//	return &csi.ControllerPublishVolumeResponse{}, nil
	return nil, status.Errorf(codes.Unimplemented, "method ControllerPublishVolume not implemented")
}

func (cs *ControllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Info("ControllerService::ControllerUnpublishVolume")
	cs.mfslog.Log(fmt.Sprintf("ControllerService::ControllerUnpublishVolume (volume_id %s)", req.VolumeId))
	//	return &csi.ControllerUnpublishVolumeResponse{}, nil
	return nil, status.Errorf(codes.Unimplemented, "method ControllerUnpublishVolume not implemented")
}

////////////////////////

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (cs *ControllerService) newVolume(volumeId string, volumeCapacity uint64, parameters map[string]string) (map[string]string, error) {
	//	cs.lock.Lock()
	//	defer cs.lock.Unlock()

	//	volumes, err := cs._loadVolumes()
	//	if err != nil {
	//		return nil, err
	//	}

	//	if vol, ok := volumes[volumeId]; ok {
	//		log.Infof("ControllerService::newVolume -- Volume %s was already created, ignoring... (capacity %d, dir %s, submount %t)", volumeId, vol.VolumeCapacity, vol.VolumeDir, vol.SubMount)
	//		return map[string]string{"VolumeDir": vol.VolumeDir}, nil
	//	}

	//	val, mountMode := parameters["mountMfs"]
	//	if mountMode {
	//		if val != "true" {
	//			mountMode = false
	//		}
	//	}

	var acquiredSize uint64
	var volumeDir string
	//	if !mountMode {
	volumeDir = path.Join(cs.ctlMount.MfsPluginPath(), volumeId)
	exists, err := pathExists(path.Join(cs.ctlMount.hostPath, volumeDir))
	if err != nil {
		return nil, err
	}
	if exists {
		currQuota, err := GetQuota(path.Join(cs.ctlMount.hostPath, volumeDir))
		if err != nil {
			return nil, err
		}
		if currQuota != volumeCapacity {
			return nil, fmt.Errorf("volume %s already exists and has different capacity from requested (current %d, requested %d)", volumeId, currQuota, volumeCapacity)
		}
	} else {
		if err := os.MkdirAll(path.Join(cs.ctlMount.hostPath, volumeDir), 0755); err != nil {
			return nil, err
		}
		acquiredSize, err = SetQuota(path.Join(cs.ctlMount.hostPath, volumeDir), volumeCapacity)
		if err != nil {
			return nil, err
		}
		_ = acquiredSize
	}
	//	} else {
	//		volumeDir = ""
	//	}

	//	volumes[volumeId] = &mfsVolume{
	//		VolumeId:       volumeId,
	//		VolumeCapacity: acquiredSize,
	//		VolumeDir:      volumeDir,
	//		SubMount:       mountMode,
	//	}
	//	err = cs._saveVolume(volumes)
	//	if err != nil {
	//		return nil, err
	//	}
	return map[string]string{"VolumeDir": volumeDir}, nil
}

func (cs *ControllerService) delVolume(volumeId string) error {
	log.Infof("ControllerService::delVolume -- Deleting directory %s", volumeId)
	volumeDir := path.Join(cs.ctlMount.MfsPluginPath(), volumeId)
	if err := os.RemoveAll(path.Join(cs.ctlMount.hostPath, volumeDir)); err != nil {
		log.Errorf("ControllerService::delVolume -- Couldn't remove directory %s, error: %s",
			volumeDir, err.Error())
		return err
	}
	return nil
}

func (cs *ControllerService) expandVolume(volumeId string, size uint64) (uint64, error) {
	volumeDir := path.Join(cs.ctlMount.MfsPluginPath(), volumeId)
	acquiredSize, err := SetQuota(path.Join(cs.ctlMount.hostPath, volumeDir), size)
	if err != nil {
		return 0, err
	}
	return acquiredSize, nil
}

/*
func (cs *ControllerService) delVolume(volumeId string) error {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	volumes, err := cs._loadVolumes()
	if err != nil {
		return err
	}
	vol, ok := volumes[volumeId]
	if !ok {
		log.Infof("ControllerService::delVolume -- Volume %s not found. Ignoring...", volumeId)
		return nil
	}
	if !vol.SubMount {
		log.Infof("ControllerService::delVolume -- Deleting directory %s", vol.VolumeDir)
		if err := os.RemoveAll(path.Join(cs.ctlMount.hostPath, vol.VolumeDir)); err != nil {
			log.Errorf("ControllerService::delVolume -- Couldn't remove directory %s, error: %s",
				vol.VolumeDir, err.Error())
			return err
		}
	} else {
		log.Infof("ControllerService::delVolume -- Deleting mfs mount. no filesystem actions are required...")
	}
	delete(volumes, volumeId)
	err = cs._saveVolume(volumes)
	if err != nil {
		return err
	}
	return nil
}

func (cs *ControllerService) expandVolume(volumeId string, size uint64) (uint64, error) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	volumes, err := cs._loadVolumes()
	if err != nil {
		return 0, err
	}
	vol, ok := volumes[volumeId]
	if !ok {
		return 0, errors.New("ControllerService::expandVolume -- Trying to expand missing volume")
	}
	if vol.SubMount {
		return 0, errors.New("ControllerService::expandVolume -- Trying to expand sub mount volume")
	}
	if vol.VolumeCapacity >= size {
		log.Infof("ControllerService::expandVolume -- Requested size is already satisfied, ignoring...")
		return vol.VolumeCapacity, nil
	}
	acquiredSize, err := SetQuota(path.Join(cs.ctlMount.hostPath, vol.VolumeDir), size)
	if err != nil {
		return 0, err
	}
	vol.VolumeCapacity = acquiredSize

	volumes[volumeId] = vol
	err = cs._saveVolume(volumes)
	if err != nil {
		return 0, err
	}
	return acquiredSize, nil
}
*/

//////////////////////

// getRequestCapacity extracts the storage size from the given capacity
// range. If the capacity range is not satisfied it returns the default volume
// size.
func getRequestCapacity(capRange *csi.CapacityRange) (int64, error) {
	reqSize := capRange.RequiredBytes
	maxSize := capRange.LimitBytes
	var capacity int64 = 0

	if reqSize == 0 && maxSize == 0 {
		return 0, fmt.Errorf("getRequestCapacity: RequredBytes or LimitBytes must be provided")
	}
	if reqSize < 0 || maxSize < 0 {
		return 0, fmt.Errorf("getRequestCapacity: RequredBytes and LimitBytes can't be negative")
	}
	if reqSize == 0 {
		capacity = maxSize
	} else {
		capacity = reqSize
	}
	return capacity, nil
}
