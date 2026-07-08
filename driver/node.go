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
	"context"
	"math/rand"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type NodeService struct {
	csi.UnimplementedNodeServer
	Service

	mountPointsCount int
	mountPoints      []*mfsHandler
	nodeId           string
}

var _ csi.NodeServer = &NodeService{}

var nodeCapabilities = []csi.NodeServiceCapability_RPC_Type{
	//csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	// csi.NodeServiceCapability_RPC_VOLUME_CONDITION,
	csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	csi.NodeServiceCapability_RPC_VOLUME_MOUNT_GROUP,
}

func NewNodeService(mfsmaster string, mfsmaster_port int, rootPath, pluginDataPath, nodeId, mfsMountOptions string, mountPointsCount int) (*NodeService, error) {
	log.Infof("NewNodeService creation (mfsmaster %s, rootDir %s, pluginDataDir %s, nodeId %s, mountPointsCount %d)", mfsmaster, rootPath, pluginDataPath, nodeId, mountPointsCount)

	mountPoints := make([]*mfsHandler, mountPointsCount)
	for i := 0; i < mountPointsCount; i++ {
		mountPoints[i] = NewMfsHandler(mfsmaster, mfsmaster_port, rootPath, pluginDataPath, nodeId, mfsMountOptions, i, mountPointsCount)
		if err := mountPoints[i].MountMfs(); err != nil {
			return nil, err
		}
	}
	if MfsLog {
		mountPoints[0].SetMfsLogging()
	}

	ns := &NodeService{
		mountPointsCount: mountPointsCount,
		mountPoints:      mountPoints,
		nodeId:           nodeId,
	}
	return ns, nil
}

// NodeStageVolume mounts the MooseFS backing directory for a volume once per
// node, independently of any individual pod. For dynamically-provisioned
// volumes (the common case), this performs a direct mfsmount of the volume's
// sub-path onto req.StagingTargetPath -- executed in the host's mount/PID
// namespaces when HostNamespaceMount is enabled, so its lifetime is
// independent of the csi-moosefs-node container (see MountMfsVolumeStaged).
// NodePublishVolume then simply bind-mounts from this staging path into each
// pod's target, which is what makes those bind-mounts durable across
// csi-moosefs-node restarts (see https://github.com/moosefs/moosefs-csi/issues/32).
//
// Statically-provisioned volumes (identified by the "mfsSubDir" volume
// context key) bypass staging and keep using the legacy shared pool-mount
// bind path in NodePublishVolume, unchanged.
func (ns *NodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	log.Infof("NodeStageVolume - VolumeId: %s, StagingTargetPath: %s, VolumeContext: %v", req.GetVolumeId(), req.GetStagingTargetPath(), req.GetVolumeContext())
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume: VolumeId must be provided")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume: StagingTargetPath must be provided")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume: VolumeCapability must be provided")
	}

	if _, found := req.GetVolumeContext()["mfsSubDir"]; found {
		log.Infof("NodeStageVolume - VolumeId: %s uses mfsSubDir, skipping staging (handled directly in NodePublishVolume)", req.VolumeId)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	handler, err := ns.pickHandlerFromVolumeId(req.VolumeId)
	if err != nil {
		return nil, err
	}

	mfsSubPath := handler.MfsPathToVolume(req.VolumeId)
	options := req.VolumeCapability.GetMount().GetMountFlags()

	if err := handler.MountMfsVolumeStaged(mfsSubPath, req.StagingTargetPath, options...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if mountVol := req.VolumeCapability.GetMount(); mountVol != nil && mountVol.VolumeMountGroup != "" {
		if gid, err := strconv.ParseInt(mountVol.VolumeMountGroup, 10, 64); err == nil {
			log.Infof("NodeStageVolume - Parsed fsGroup: %d", gid)
			if err := handler.applyFSGroupPermissions(req.StagingTargetPath, gid); err != nil {
				log.Errorf("NodeStageVolume - Failed to apply fsGroup permissions: %v", err)
				// Don't fail staging for permission errors, log and continue
			}
		} else {
			log.Errorf("NodeStageVolume - Failed to parse fsGroup '%s': %v", mountVol.VolumeMountGroup, err)
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unmounts the staging mount created by NodeStageVolume.
// Kubelet only calls this once no pod on this node references the volume
// anymore, so no additional reference counting is required here.
func (ns *NodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log.Infof("NodeUnstageVolume - VolumeId: %s, StagingTargetPath: %s", req.GetVolumeId(), req.GetStagingTargetPath())
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume: VolumeId must be provided")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume: StagingTargetPath must be provided")
	}

	if err := ns.mountPoints[0].UnmountStaged(req.StagingTargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Infof("NodePublishVolume - VolumeId: %s, Readonly: %v, VolumeContext %v, PublishContext %v, VolumeCapability %v TargetPath %s StagingTargetPath %s", req.GetVolumeId(), req.GetReadonly(), req.GetVolumeContext(), req.GetPublishContext(), req.GetVolumeCapability(), req.GetTargetPath(), req.GetStagingTargetPath())
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: VolumeId must be provided")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: TargetPath must be provided")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume: VolumeCapability must be provided")
	}

	target := req.TargetPath
	options := req.VolumeCapability.GetMount().MountFlags
	if req.GetReadonly() {
		options = append(options, "ro")
	}

	handler, err := ns.pickHandler(req.GetVolumeContext(), req.GetPublishContext())
	if err != nil {
		return nil, err
	}

	if subDir, found := req.GetVolumeContext()["mfsSubDir"]; found {
		// Legacy/static path: bind directly from the shared per-node pool
		// mount, as before. This path is not durable across
		// csi-moosefs-node restarts; see NodeStageVolume for the fixed
		// dynamic-provisioning path.
		var fsGroup *int64
		if mountVol := req.VolumeCapability.GetMount(); mountVol != nil && mountVol.VolumeMountGroup != "" {
			if gid, err := strconv.ParseInt(mountVol.VolumeMountGroup, 10, 64); err == nil {
				fsGroup = &gid
				log.Infof("NodePublishVolume - Parsed fsGroup: %d", gid)
			} else {
				log.Errorf("NodePublishVolume - Failed to parse fsGroup '%s': %v", mountVol.VolumeMountGroup, err)
			}
		}
		if err := handler.BindMountWithFSGroup(subDir, target, fsGroup, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Standard dynamically-provisioned path: bind from the per-volume
	// staging mount created in NodeStageVolume. Because that staging mount
	// is independent of this container's lifecycle, this bind mount (and
	// its propagation into the host mount namespace) remains valid across
	// csi-moosefs-node restarts.
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.FailedPrecondition, "NodePublishVolume: StagingTargetPath not provided; NodeStageVolume must be called first")
	}
	if err := handler.BindMountAbs(req.GetStagingTargetPath(), target, options...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Infof("NodeUnpublishVolume - VolumeId: %s, TargetPath: %s)", req.GetVolumeId(), req.GetTargetPath())
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume: Volume Id must be provided")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume: Target Path must be provided")
	}

	found, err := ns.mountPoints[0].VolumeExist(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	} else if !found {
		found, err = ns.mountPoints[0].MountVolumeExist(req.VolumeId)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if !found {
			return nil, status.Errorf(codes.NotFound, "NodeUnpublishVolume: volume %s not found", req.VolumeId)
		}
	}
	if err = ns.mountPoints[0].BindUMount(req.TargetPath); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	log.Infof("NodeGetInfo")
	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeId,
	}, nil
}

func (ns *NodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log.Infof("NodeGetCapabilities")
	var caps []*csi.NodeServiceCapability
	for _, capa := range nodeCapabilities {
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

/*
func (ns *NodeService) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	log.Infof("NodeService::NodeGetVolumeStats (volume_id %s, volume_path %s, staging_path %s)",
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
*/
//////////////

// Stop implements driver.Stopper. It is invoked by StartService during
// graceful shutdown (SIGTERM/SIGINT) to unmount the shared per-node
// pool mount(s) so that the MooseFS master sees clean session closes.
// Per-volume staging mounts are intentionally NOT unmounted here:
// they live in the host mount namespace (see MountMfsVolumeStaged) and
// must keep serving application pods across csi-moosefs-node restarts.
// Errors from individual pool unmounts are logged but do not abort the
// shutdown, so one bad mount never blocks container termination.
func (ns *NodeService) Stop() {
	for i, mp := range ns.mountPoints {
		if mp == nil {
			continue
		}
		if err := mp.UnmountPool(); err != nil {
			log.Errorf("NodeService::Stop - failed to unmount pool mount %d (%s): %v", i, mp.hostMountPath, err)
		}
	}
}

// pickHandler - Returns proper handler. Currently picks random mfs handler.
func (ns *NodeService) pickHandler(volumeContext map[string]string, publishContext map[string]string) (*mfsHandler, error) {
	if ns.mountPointsCount <= 0 {
		return nil, status.Error(codes.Internal, "pickHandler: there is no mfs handlers")
	}
	return ns.mountPoints[rand.Uint32()%uint32(ns.mountPointsCount)], nil
}

// pickHandlerFromVolumeId - Unimplemented, always picks first handler.
func (ns *NodeService) pickHandlerFromVolumeId(volumeId string) (*mfsHandler, error) {
	if ns.mountPointsCount <= 0 {
		return nil, status.Error(codes.Internal, "pickHandlerFromVolumeId: there is no mfs handlers")
	}
	return ns.mountPoints[0], nil
}
