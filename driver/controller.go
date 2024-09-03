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
	"context"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

var controllerCapabilities = []csi.ControllerServiceCapability_RPC_Type{
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	//	csi.ControllerServiceCapability_RPC_PUBLISH_READONLY,
}

type ControllerService struct {
	//	csi.UnimplementedControllerServer
	csi.ControllerServer
	Service

	ctlMount *mfsHandler
}

var _ csi.ControllerServer = &ControllerService{}

func NewControllerService(mfsmaster string, mfsmaster_port int, rootPath, pluginDataPath, mfsMountOptions string) (*ControllerService, error) {
	log.Infof("NewControllerService creation - mfsmaster %s, rootDir %s, pluginDataDir %s)", mfsmaster, rootPath, pluginDataPath)

	ctlMount := NewMfsHandler(mfsmaster, mfsmaster_port, rootPath, pluginDataPath, "controller", mfsMountOptions)
	if err := ctlMount.MountMfs(); err != nil {
		return nil, err
	}
	if MfsLog {
		ctlMount.SetMfsLogging()
	}
	return &ControllerService{ctlMount: ctlMount}, nil
}

// CreateVolume creates a new volume from the given request. The function is idempotent.
func (cs *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log.Infof("CreateVolume - Name: %s", req.Name)
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: Name must be provided")
	}
	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: Volume capabilities must be provided")
	}
	requestedQuota, err := getRequestCapacity(req.CapacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			return nil, status.Error(codes.InvalidArgument, "CreateVolume: Block storage not supported")
		}
	}

	if req.VolumeContentSource != nil {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume: VolumeContentSource not supported")
	}

	volumeId := req.Name
	exists, err := cs.ctlMount.VolumeExist(volumeId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	var acquiredSize int64
	if exists {
		currQuota, err := cs.ctlMount.GetQuota(volumeId)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if currQuota != requestedQuota {
			return nil, status.Errorf(codes.AlreadyExists, "CreateVolume: volume %s already exists and has different capacity from requested (current %d, requested %d)",
				volumeId, currQuota, requestedQuota)
		}
	} else {
		acquiredSize, err = cs.ctlMount.CreateVolume(volumeId, requestedQuota)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if acquiredSize != requestedQuota {
			log.Warningf("CreateVolume - requested %d bytes, got %d", requestedQuota, acquiredSize)
		}
	}
	if len(req.Parameters) != 0 {
		return nil, status.Errorf(codes.Internal, "CreateVolume: Plugin parameters are not supported")
	}
	/*
		volumeContext := req.GetParameters()
		if volumeContext == nil {
			volumeContext = make(map[string]string)
		}
		mfsVolumePath := cs.ctlMount.MfsPathToVolume(volumeId)
		volumeContext["mfsVolumePath"] = mfsVolumePath

		resp := &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      req.GetName(),
				CapacityBytes: acquiredSize,
				VolumeContext: volumeContext,
			},
		}
		return resp, nil

	*/
	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      req.GetName(),
			CapacityBytes: acquiredSize,
		},
	}
	return resp, nil
}

func (cs *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log.Infof("DeleteVolume - VolumeId: %s", req.VolumeId)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume: VolumeId must be provided")
	}

	exists, err := cs.ctlMount.VolumeExist(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !exists {
		return &csi.DeleteVolumeResponse{}, nil
	}
	if err := cs.ctlMount.DeleteVolume(req.VolumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume: VolumeId must be provided")
	}
	size, err := getRequestCapacity(req.CapacityRange)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	log.Infof("ControllerExpandVolume - VolumeId: %s, size: %d)", req.VolumeId, size)

	exists, err := cs.ctlMount.VolumeExist(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !exists {
		return nil, status.Error(codes.NotFound, "ControllerExpandVolume: Volume not found")
	}

	acquiredSize, err := cs.ctlMount.SetQuota(req.VolumeId, size)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if acquiredSize != size {
		log.Warningf("ControllerExpandVolume - requested %d bytes, got %d", size, acquiredSize)
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         acquiredSize,
		NodeExpansionRequired: false,
	}, nil
}

func (cs *ControllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	log.Infof("ValidateVolumeCapabilities - VolumeId: %s", req.VolumeId)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities: VolumeId must be provided")
	}
	if len(req.VolumeCapabilities) == 0 || req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities: VolumeCapabilities must be provided")
	}

	if exists, err := cs.ctlMount.VolumeExist(req.VolumeId); err != nil {
		return nil, err
	} else if !exists {
		return nil, status.Errorf(codes.NotFound, "ValidateVolumeCapabilities: Volume %s not found", req.VolumeId)
	}

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
	log.Infof("ControllerGetCapabilities")
	var caps []*csi.ControllerServiceCapability
	for _, capa := range controllerCapabilities {
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

func (cs *ControllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Infof("ControllerPublishVolume - VolumeId: %s NodeId: %s VolumeContext: %v", req.VolumeId, req.NodeId, req.VolumeContext)
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume: VolumeId must be provided")
	}
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume: NodeId must be provided")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume: VolumeCapability capabilities must be provided")
	}

	if req.VolumeCapability.GetBlock() != nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume: Block storage not supported")
	}
	publishContext := make(map[string]string)
	if req.Readonly {
		publishContext["readonly"] = "true"
	}
	// dynamic or static and existing volume
	if len(req.GetVolumeContext()) == 0 {
		exists, err := cs.ctlMount.VolumeExist(req.VolumeId)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if !exists {
			return nil, status.Errorf(codes.NotFound, "ControllerPublishVolume: Volume %s not found", req.VolumeId)
		} else {
			return &csi.ControllerPublishVolumeResponse{PublishContext: publishContext}, nil
		}
	}
	create, found := req.VolumeContext["create_on_publish"]
	do_create := (found && create == "true")
	_, found = req.VolumeContext["mfsSubDir"]
	if found {
		if do_create {
			return nil, status.Errorf(codes.InvalidArgument, "ControllerPublishVolume: VolumeContext contain both 'create' and 'mfsSubDir'")
		} else {
			cs.ctlMount.CreateMountVolume(req.VolumeId)
			return &csi.ControllerPublishVolumeResponse{PublishContext: publishContext}, nil
		}
	}

	if exists, err := cs.ctlMount.VolumeExist(req.VolumeId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else if exists {
		return &csi.ControllerPublishVolumeResponse{PublishContext: publishContext}, nil
	}

	if do_create {
		if _, err := cs.ctlMount.CreateVolume(req.VolumeId, 0); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.ControllerPublishVolumeResponse{PublishContext: publishContext}, nil
	} else {
		return nil, status.Errorf(codes.NotFound, "ControllerPublishVolume: Volume %s not found, 'create_on_publish' set to false'", req.VolumeId)
	}
}

func (cs *ControllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Infof("ControllerUnpublishVolume - VolumeId: %s, NodeId: %s", req.VolumeId, req.NodeId)
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume: VolumeId must be provided")
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

//////////////////////

// getRequestCapacity extracts the storage size from the given capacity
// range. If the capacity range is not satisfied it returns the default volume
// size.
func getRequestCapacity(capRange *csi.CapacityRange) (int64, error) {
	// todo(ad): fix default value
	if capRange == nil {
		return 1 << 31, nil
	}
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

//////////
/*
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
*/
