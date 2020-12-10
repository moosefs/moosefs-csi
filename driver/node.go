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
	"io/ioutil"
	"math/rand"
	"path"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type NodeService struct {
	csi.NodeServer
	*IdentityService
	*PluginService

	mountPointsCount int
	mountPoints      []*MfsMountPoint
	nodeID           string
	mfslog           *MfsLog
}

func (ns *NodeService) Register(srv *grpc.Server) {
	log.Infof("NodeService::Register")
	ns.IdentityService.Register(srv)
	csi.RegisterNodeServer(srv, ns)
}

func NewNodeService(mfsmaster, rootDir, pluginDataDir, nodeID string, mountPointsCount int) (*NodeService, error) {
	ns := &NodeService{
		mountPointsCount: mountPointsCount,
		mountPoints:      make([]*MfsMountPoint, mountPointsCount),
		nodeID:           nodeID,
		mfslog: &MfsLog{
			logfile: path.Join(fmt.Sprintf("/mnt/mount_node_%s_%02d", nodeID, 0),
				pluginDataDir, fmt.Sprintf("logz_%s", nodeID)),
			active: false,
		},
	}
	for i := 0; i < mountPointsCount; i++ {
		ns.mountPoints[i], _ = NewMfsMountPoint(mfsmaster, rootDir, fmt.Sprintf("/mnt/mount_node_%s_%02d", nodeID, i), pluginDataDir)
		if err := ns.mountPoints[i].Mount(); err != nil {
			return nil, err
		}
	}
	return ns, nil
}

func (ns *NodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	log.Infof("NodeServer::NodePublishVolume (volume_id %s, target_path %s, volumecontext %v)", req.VolumeId, req.TargetPath, req.GetVolumeContext())
	ns.mfslog.Log(fmt.Sprintf("NodeServer::NodePublishVolume (volume_id %s, target_path %s)", req.VolumeId, req.TargetPath))

	source, ok := req.GetVolumeContext()["VolumeDir"]
	log.Infof("NodeService::NodePublishVolume -- source here is %s", source)

	if !ok {
		sub_source, ok := req.GetVolumeContext()["mfsSubFolder"]
		log.Infof("NodeService::NodePublishVolume -- sub_source here is %s", sub_source)
		if !ok {
			log.Errorf("NodeService::NodePublishVolume -- VolumeContext doesn't contain 'VolumeDir' and 'mfsSubFolder' fields. Aborting...")
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume 'VolumeDir' and 'mfsSubFolder' not found in VolumeContext")
		}
		source = sub_source
	} else {
		log.Infof("NodeService::NodePublishVolume -- source was found and idk (%s)", source)
		_, ok := req.GetVolumeContext()["mfsSubFolder"]
		if ok {
			log.Errorf("NodeService::NodePublishVolume -- VolumeContext contain both 'VolumeDir' and 'mfsSubFolder' fields. Aborting...")
			return nil, status.Error(codes.Internal, "NodePublishVolume both 'VolumeDir' and 'mfsSubFolder' found in VolumeContext")
		}
	}
	log.Infof("NodeService::NodePublishVolume -- source here is %s", source)

	target := req.TargetPath
	options := req.VolumeCapability.GetMount().MountFlags
	if req.Readonly {
		options = append(options, "ro")
	}

	if err := ns.bindMount(source, target, options...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Infof("NodeServer::NodeUnpublishVolume (volume_id %s, target_path %s)", req.VolumeId, req.TargetPath)
	ns.mfslog.Log(fmt.Sprintf("NodeServer::NodeUnpublishVolume (volume_id %s, target_path %s)", req.VolumeId, req.TargetPath))

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	err := ns.unbindMount(req.TargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	log.Infof("NodeServer::NodeGetInfo")

	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
	}, nil
}

func (ns *NodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log.Infof("NodeServer::NodeGetCapabilities")

	var caps []*csi.NodeServiceCapability
	for _, capa := range []csi.NodeServiceCapability_RPC_Type{
		//csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		// csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
		//		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	} {
		caps = append(caps, &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: capa,
				},
			},
		})
	}

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (ns *NodeService) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log.Infof("NodeServer::NodeGetVolumeStats (volume_id %s, volume_path %s, staging_path %s)",
		req.VolumeId, req.VolumePath, req.StagingTargetPath)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats: VolumeId must be provided")
	}
	if req.VolumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats: VolumePath must be provided")
	}

	cond := false
	_, err := ioutil.ReadDir(req.VolumePath)
	if err != nil {
		log.Infof("%s %s corrupted", req.VolumeId, req.VolumePath)
		cond = true
	} else {
		log.Infof("%s %s NOT corrupted", req.VolumeId, req.VolumePath)
	}
	return &csi.NodeGetVolumeStatsResponse{VolumeCondition: &csi.VolumeCondition{
		Abnormal: cond,
		Message:  "",
	}}, nil
}

func (ns *NodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log.Info("NodeServer::NodeStageVolume")
	ns.mfslog.Log(fmt.Sprintf("NodeServer::NodeStageVolume (volume_id %s)", req.VolumeId))
	return &csi.NodeStageVolumeResponse{}, nil
	//	return nil, status.Errorf(codes.Unimplemented, "method NodeStageVolume not implemented")
}

func (ns *NodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log.Info("NodeServer::NodeUnstageVolume")
	ns.mfslog.Log(fmt.Sprintf("NodeServer::NodeUnstageVolume (volume_id %s)", req.VolumeId))

	return &csi.NodeUnstageVolumeResponse{}, nil
	//	return nil, status.Errorf(codes.Unimplemented, "method NodeUnstageVolume not implemented")
}

func (ns *NodeService) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	log.Info("NodeServer::NodeExpandVolume")
	return nil, status.Errorf(codes.Unimplemented, "method NodeExpandVolume not implemented")
}

//////////////

func (ns *NodeService) bindMount(mfsSource string, target string, options ...string) error {
	mounter := Mounter{}

	ismounted, err := mounter.IsMounted(target)
	if err != nil {
		return err
	}
	if !ismounted {
		mountId := rand.Intn(ns.mountPointsCount)
		source := path.Join(ns.mountPoints[mountId].hostPath, mfsSource)
		log.Infof("NodeService::bindMount -- mounting %s (%s + %s) to %s", source, ns.mountPoints[mountId].hostPath, mfsSource, target)
		if err := mounter.Mount(source, target, fsType, append(options, "bind")...); err != nil {
			return err
		}
	} else {
		log.Infof("NodeService::bindMount -- target %s is already mounted", target)
	}
	return nil
}

func (ns *NodeService) unbindMount(target string) error {
	mounter := Mounter{}

	mounted, err := mounter.IsMounted(target)
	if err != nil {
		return err
	}

	if mounted {
		err := mounter.UMount(target)
		if err != nil {
			return err
		}
	} else {
		log.Infof("NodeService::unbindMount -- target %s was already unmounted", target)
	}
	return nil
}
