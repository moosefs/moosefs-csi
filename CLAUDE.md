# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Container Storage Interface (CSI) driver for MooseFS, a distributed file system. The driver enables Kubernetes to provision and manage persistent volumes backed by MooseFS storage.

## Build Commands

**Compile the binary:**
```bash
make compile
```

**Build development Docker image:**
```bash
make build-dev
```

**Build production Docker image:**
```bash
make build-prod
```

**Cross-platform build with buildx:**
```bash
make dev-buildx    # For development
make prod-buildx   # For production
```

**Clean build artifacts:**
```bash
make clean
```

## Driver Architecture

The CSI driver operates in two distinct modes, each running as separate processes:

### Controller Service (driver/controller.go)
- Handles volume lifecycle operations: create, delete, expand
- Manages volume publishing/unpublishing to nodes
- Runs as a single instance in the cluster
- Maintains a MooseFS mount at `/mnt/controller` for volume operations
- Supports dynamic provisioning with quotas and static provisioning

### Node Service (driver/node.go)
- Handles volume mounting/unmounting on individual nodes
- Manages multiple MooseFS client connections (pool-based)
- Each node runs one instance with configurable mount point count
- Mount points: `/mnt/${nodeId}[_${mountId}]`

### Core Components

**mfsHandler (driver/mfs_handler.go):**
- Abstraction layer for MooseFS operations
- Handles mfsmount/mfsumount commands
- Manages quotas via mfsgetquota/mfssetquota
- Creates volume directories and mount volumes

**Service Interface (driver/service.go):**
- gRPC server setup and request routing
- Common driver initialization and logging
- CSI endpoint management (Unix sockets)

## Key Configuration

The driver is configured via `deploy/csi-moosefs-config.yaml`:
- `master_host`: MooseFS master server address
- `master_port`: MooseFS master port (default: 9421)
- `k8s_root_dir`: Root directory on MooseFS for all volumes
- `mount_count`: Number of pre-created MooseFS clients per node

## Volume Management

**Dynamic Provisioning:**
- Volumes created in `${k8s_root_dir}/${driver_working_dir}/volumes/`
- Quotas enforced via MooseFS quota system
- Support for volume expansion (increase only)

**Static Provisioning:**
- Mount any MooseFS directory using `mfsSubDir` parameter
- No quota management for static volumes

**Mount Volumes:**
- Special volumes for mounting existing MooseFS directories
- Created in `${k8s_root_dir}/${driver_working_dir}/mount_volumes/`

## Dependencies

- Go 1.14+
- github.com/container-storage-interface/spec v1.5.0 (updated for fsGroup support)
- google.golang.org/grpc v1.36.0
- github.com/sirupsen/logrus v1.8.0
- gopkg.in/natefinch/lumberjack.v2 v2.0.0 (log rotation)

## Deployment Structure

The driver uses two main manifests:
- `deploy/csi-moosefs-config.yaml`: Configuration values
- `deploy/csi-moosefs.yaml`: Pod specs for controller and node services

Version compatibility is maintained between Kubernetes versions and driver releases (see README).

## FSGroup Support

The driver implements full Kubernetes CSI fsGroup support to ensure proper UID/GID handling:

**Features:**
- `VOLUME_MOUNT_GROUP` node capability advertised
- `fsGroupPolicy: File` configured in CSIDriver spec
- Automatic application of fsGroup permissions during NodePublishVolume
- Volume directories get group ownership set to fsGroup with 0775 permissions

**Implementation Details:**
- fsGroup extracted from `VolumeCapability.MountVolume.VolumeMountGroup`
- Applied via `applyFSGroupPermissions()` in driver/mfs_handler.go:345
- Permissions changed using `os.Chown()` and `os.Chmod()` syscalls
- Non-fatal: permission errors logged but don't fail the mount operation

**Usage:**
Set fsGroup in Pod SecurityContext - the driver will automatically apply it:
```yaml
spec:
  securityContext:
    fsGroup: 1000
```