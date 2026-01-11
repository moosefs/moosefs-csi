# FSGroup Support Test

This example demonstrates the MooseFS CSI driver's fsGroup support, which automatically applies proper UID/GID permissions to volumes.

## What This Tests

- Pod running as non-root user (UID 1000) 
- fsGroup set to 1000 in Pod SecurityContext
- CSI driver automatically applies group ownership to volume
- Container can write to the volume without permission errors

## Usage

1. Apply the PVC:
   ```bash
   kubectl apply -f pvc.yaml
   ```

2. Apply the Pod:
   ```bash
   kubectl apply -f pod.yaml
   ```

3. Verify the Pod can write to the volume:
   ```bash
   kubectl exec test-fsgroup-pod -- touch /data/test-file
   kubectl exec test-fsgroup-pod -- ls -la /data/
   ```

## Expected Results

- The volume directory should have group ownership set to 1000
- The container running as UID 1000 should be able to create files
- Files created should be owned by UID 1000 and GID 1000

## Verification Commands

Check permissions on the volume:
```bash
kubectl exec test-fsgroup-pod -- ls -ld /data
```

Test write access:
```bash
kubectl exec test-fsgroup-pod -- echo "Hello World" > /data/test.txt
kubectl exec test-fsgroup-pod -- cat /data/test.txt
```

## Cleanup

```bash
kubectl delete -f pod.yaml
kubectl delete -f pvc.yaml
```

## How It Works

1. **CSI Driver Configuration**: `fsGroupPolicy: File` in CSIDriver spec enables fsGroup support
2. **Node Capability**: `VOLUME_MOUNT_GROUP` capability tells Kubernetes the driver handles fsGroup
3. **Permission Application**: During `NodePublishVolume`, the driver:
   - Extracts fsGroup from `VolumeCapability.MountVolume.VolumeMountGroup`
   - Calls `applyFSGroupPermissions()` to set group ownership 
   - Changes directory permissions to 0775 (group writable)
4. **Result**: Pod processes can write to the volume even when running as non-root