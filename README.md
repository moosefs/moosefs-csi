# Container Storage Interface (CSI) driver for MooseFS
[Container storage interface](https://github.com/container-storage-interface/spec) is an [industry standard](https://github.com/container-storage-interface/spec/blob/master/spec.md) that enables storage vendors to develop a plugin once and have it work across a number of container orchestration systems.

[MooseFS](https://moosefs.com) is a petabyte Open-Source distributed file system. It aims to be fault-tolerant, highly available, highly performing, scalable general-purpose network distributed file system for data centers.

MooseFS source code can be found [on GitHub](https://github.com/moosefs/moosefs).

---

*Note that on each node a pool of MooseFS Clients that are available for use by containers is created. By default the number of MooseFS Clients in the pool is `1`.*

## Installation on Kubernetes

### Prerequisites

* MooseFS Cluster up and running

* `--allow-privileged=true` flag set for both API server and kubelet (default value for kubelet is `true`)

### Deployment

1. Complete `deploy/kubernetes/moosefs-csi-config.yaml` configuration file with your settings:
    * `master_host` – IP address of your MooseFS Master Server(s). It is an equivalent to `-H master_host` or `-o mfsmaster=master_host` passed to MooseFS Client.
    * `k8s_root_dir` – each mount's root directory on MooseFS. Each path is relative to this one. Equivalent to `-S k8s_root_dir` or `-o mfssubfolder=k8s_root_dir` passed to MooseFS Client.
    * `pv_working_dir` – a directory inside MooseFS where persistent volumes are stored (actual path is: `k8s_root_dir` + `pv_working_dir`)
    * `mount_count` – number of pre created MooseFS clients running on each node

    and apply:
    
    ```
    $ kubectl apply -f deploy/kubernetes/moosefs-csi-config.yaml
    ```

2. ConfigMap should now be created:

    ```
    $ kubectl get configmap -n kube-system
    NAME                                 DATA   AGE
    csi-moosefs-config                   5      42s
    ```

3. Deploy CSI MooseFS plugin along with CSI Sidecar Containers:

    ```
    $ kubectl apply -f deploy/kubernetes/moosefs-csi.yaml
    ```

4. Ensure that all the containers are ready, up and running

    ```
    kube@k-master:~$ kubectl get pods -n kube-system | grep csi-moosefs
    csi-moosefs-controller-0                   3/3     Running   0          44m
    csi-moosefs-node-7h4pj                     2/2     Running   0          44m
    csi-moosefs-node-8n5hj                     2/2     Running   0          44m
    csi-moosefs-node-n4prg                     2/2     Running   0          44m
    ```
   
    You should see a single `csi-moosefs-controller-x` running and `csi-moosefs-node-xxxxx` one per each node.

    You may also take a look at your MooseFS CGI Monitoring Interface ("Mounts" tab) to check if new Clients are connected – mount points: `/mnt/mount_controller` and `/mnt/mount_node_${nodeId}_{mountId}`.

### Verification

1. Create a persistent volume claim for 5 GiB:

    ```
    $ kubectl apply -f examples/kubernetes/dynamic-provisioning/pvc.yaml
    ```

2. Verify if the persistent volume claim exists and wait until it's STATUS is `Bound`:

    ```
    $ kubectl get pvc
    NAME             STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
    my-moosefs-pvc   Bound    pvc-a62451d4-0d75-4f81-bfb3-8402c59bfc25   5Gi        RWX            moosefs-storage   69m
    ```

3. After its in `Bound` state, create a sample workload that mounts the volume:

    ```
    $ kubectl apply -f examples/kubernetes/dynamic-provisioning/pod.yaml
    ```

4. Verify the storage is mounted:

    ```
    $ kubectl exec my-moosefs-pod -- df -h /data
    Filesystem                Size      Used Available Use% Mounted on
    172.17.2.80:9421          4.2T      1.4T      2.8T  33% /data
    ```
   
   You may take a look at MooseFS CGI Monitoring Interface ("Quotas" tab) to check if a quota for 5 GiB on a newly created volume directory has been set.
   
5. Clean up:

    ```
    $ kubectl delete -f examples/kubernetes/dynamic-provisioning/pod.yaml
    $ kubectl delete -f examples/kubernetes/dynamic-provisioning/pvc.yaml
    ```

## More examples and capabilities

### Volume Expansion

Volume expansion can be done by updating and applying corresponding PVC specification.

**Note:** the volume size can only be increased. Any attempts to decrease it will result in an error. It is not recommended to resize Persistent Volume MooseFS-allocated quotas via MooseFS native tools, as such changes will not be visible in your Container Orchestrator.

### Mount MooseFS inside containers

It is possible to mount any MooseFS directory inside containers using static provisioning.

1. Create a Persistent Volume (`examples/kubernetes/mount/pv.yaml`):

    ```
    kind: PersistentVolume
    apiVersion: v1
    metadata:
      name: my-moosefs-mount-pv
    spec:
      storageClassName: ""               # empty Storage Class
      capacity:
        storage: 1Gi                     # required, however does not have any effect
      accessModes:
        - ReadWriteMany
      csi:
        driver: moosefs.csi.tappest.com
        volumeHandle: submount-example   # currently does not have any effect
        volumeAttributes:
          mfsSubFolder: "/"              # subfolder to be mounted as a rootdir (inside k8s_root_dir)
    ```
   
2. Create corresponding Persistent Volume Claim (`examples/kubernetes/mount/pvc.yaml`):

    ```
    kind: PersistentVolumeClaim
    apiVersion: v1
    metadata:
      name: my-moosefs-mount-pvc
    spec:
      storageClassName: ""               # empty Storage Class
      volumeName: my-moosefs-mount-pv
      accessModes:
        - ReadWriteMany
      resources:
        requests:
          storage: 1Gi                   # at least as much as in PV, does not have any effect
    ```   
   
3. Apply both configurations:

    ```
    $ kubectl apply -f examples/kubernetes/mount/pv.yaml
    $ kubectl apply -f examples/kubernetes/mount/pvc.yaml
    ```
   
4. Verify that PVC exists and wait until it is bound to the previously created PV:

    ```
    $ kubectl get pvc
    NAME                    STATUS   VOLUME                 CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    my-moosefs-mount-pvc    Bound     my-moosefs-mount-pv   1Gi        RWX                           23m
    ```
   
5. Create a sample workload that mounts the volume:

    ```
    $ kubectl apply -f examples/kubernetes/mount/pod.yaml
    ```
   
6. Verify that the storage is mounted:

    ```
    $ kubectl exec -it my-moosefs-pod -- ls /data
    ```
   
   You should see the content of `k8s_root_dir + mfsSubFolder`.
   
7. Clean up:

    ```
    $ kubectl delete -f examples/kubernetes/mount/pod.yaml
    $ kubectl delete -f examples/kubernetes/mount/pvc.yaml
    $ kubectl delete -f examples/kubernetes/mount/pv.yaml
    ```

By using `containers[*].volumeMounts[*].subPath` field of `PodSpec` it is possible to specify a proper MooseFS subfolder using only one PV/PVC pair, without creating a new one for each subfolder:

```
kind: Deployment
apiVersion: apps/v1
metadata:
  name: my-site-app
spec:
  template:
    spec:
      containers:
        - name: my-frontend
          # ...
          volumeMounts:
            - name: my-moosefs-mount
              mountPath: "/var/www/my-site/assets/images"
              subPath: "resources/my-site/images"
            - name: my-moosefs-mount
              mountPath: "/var/www/my-site/assets/css"
              subPath: "resources/my-site/css"
      volumes:
        - name: my-moosefs-mount
          persistentVolumeClaim:
            claimName: my-moosefs-mount-pvc
```

## Version Compatibility

| Kubernetes | MooseFS CSI Driver |
|:---:|:---:|
| `v1.18.5` | `v0.9.0`|

## License
[Apache v2 license](https://www.apache.org/licenses/LICENSE-2.0)

## Code of conduct
Participation in this project is governed by [Kubernetes/CNCF code of conduct](https://github.com/kubernetes/community/blob/master/code-of-conduct.md)