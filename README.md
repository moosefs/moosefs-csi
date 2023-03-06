# Container Storage Interface (CSI) driver for MooseFS

[Container storage interface](https://github.com/container-storage-interface/spec) is an [industry standard](https://github.com/container-storage-interface/spec/blob/master/spec.md) that enables storage vendors to develop a plugin once and have it work across a number of container orchestration systems.

[MooseFS](https://moosefs.com) is a petabyte Open-Source distributed file system. It aims to be fault-tolerant, highly available, highly performing, scalable general-purpose network distributed file system for data centers.

MooseFS source code can be found [on GitHub](https://github.com/moosefs/moosefs).

---

*Note that a pool of MooseFS Clients that are available for use by containers is created on each node. By default the number of MooseFS Clients in the pool is `1`.*

## Installation on Kubernetes

### Prerequisites

* MooseFS Cluster up and running

* `--allow-privileged=true` flag set for both API server and kubelet (default value for kubelet is `true`)

### **Deployment**

1.  Complete `deploy/kubernetes/csi-moosefs-config.yaml` configuration file with your settings:

    * `master_host` – domain name (**recommended**) or IP address of your MooseFS Master Server(s). It is an equivalent to `-H master_host` or `-o mfsmaster=master_host` passed to MooseFS Client.
    * `master_port` – port number of your MooseFS Master Server. It is an equivalent to `-P master_port` or `-o mfsport=master_port` passed to MooseFS Client.
    * `k8s_root_dir` – each mount's root directory on MooseFS. Each path is relative to this one. Equivalent to `-S k8s_root_dir` or `-o mfssubfolder=k8s_root_dir` passed to MooseFS Client.
    * `driver_working_dir` – a driver working directory inside MooseFS where persistent volumes, logs and metadata is stored (actual path is: `k8s_root_dir/driver_working_dir`)
    * `mount_count` – number of pre created MooseFS clients running on each node
    and apply:
    * `mfs_logging` – driver can create logs from each component in `k8s_root_dir/driver_working_dir/logs` directory. Boolean `"true"`/`"false"` value.

    ```
    $ kubectl apply -f deploy/kubernetes/csi-moosefs-config.yaml
    ```

2.  ConfigMap should now be created:

    ```
    $ kubectl get configmap -n kube-system
    NAME                                 DATA   AGE
    csi-moosefs-config                   6      42s
    ```

3.  Update `deploy/kubernetes/csi-moosefs.yaml` file with the image that uses required MooseFS or MooseFS Pro version and MooseFS CSI Plugin version. Default images are the latest version of the plugin and the latest version of MooseFS (Community):

    * Find plugin named `csi-moosefs-plugin`
    * Update the `image` version suffix in the plugin's section accordingly, for example:
        * `0.9.4-3.0.117` – for plugin version 0.9.4 and MooseFS Community 3.0.117
        * `0.9.4-4.44.4-pro` – for plugin version 0.9.4 and MooseFS Pro 4.44.4

      You can find a complete list of available images at: \
      https://registry.moosefs.com/v2/moosefs-csi-plugin/tags/list.

      **Note there are two occurrences of `csi-moosefs-plugin` in `csi-moosefs.yaml` file and it is necessary to update the image version in both places of the file.**

4.  Deploy CSI MooseFS plugin along with CSI Sidecar Containers:

    ```
    $ kubectl apply -f deploy/kubernetes/csi-moosefs.yaml
    ```

5.  Ensure that all the containers are ready, up and running

    ```
    kube@k-master:~$ kubectl get pods -n kube-system | grep csi-moosefs
    csi-moosefs-controller-0                   4/4     Running   0          44m
    csi-moosefs-node-7h4pj                     2/2     Running   0          44m
    csi-moosefs-node-8n5hj                     2/2     Running   0          44m
    csi-moosefs-node-n4prg                     2/2     Running   0          44m
    ```
   
    You should see a single `csi-moosefs-controller-x` running and `csi-moosefs-node-xxxxx` one per each node.

    You may also take a look at your MooseFS CGI Monitoring Interface ("Mounts" tab) to check if new Clients are connected – mount points: `/mnt/controller` and `/mnt/${nodeId}[_${mountId}]`.

### **Verification**

1.  Create a persistent volume claim for 5 GiB:

    ```
    $ kubectl apply -f examples/kubernetes/dynamic-provisioning/pvc.yaml
    ```

2.  Verify if the persistent volume claim exists and wait until it's STATUS is `Bound`:

    ```
    $ kubectl get pvc
    NAME             STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
    my-moosefs-pvc   Bound    pvc-a62451d4-0d75-4f81-bfb3-8402c59bfc25   5Gi        RWX            moosefs-storage   69m
    ```

3.  After its in `Bound` state, create a sample workload that mounts the volume:

    ```
    $ kubectl apply -f examples/kubernetes/dynamic-provisioning/pod.yaml
    ```

4.  Verify the storage is mounted:

    ```
    $ kubectl exec my-moosefs-pod -- df -h /data
    Filesystem                Size      Used Available Use% Mounted on
    172.17.2.80:9421          4.2T      1.4T      2.8T  33% /data
    ```
   
    You may take a look at MooseFS CGI Monitoring Interface ("Quotas" tab) to check if a quota for 5 GiB on a newly created volume directory has been set. Dynamically provisioned volumes are stored on MooseFS in `k8s_root_dir/driver_working_dir/volumes` directory.
   
5.  Clean up:

    ```
    $ kubectl delete -f examples/kubernetes/dynamic-provisioning/pod.yaml
    $ kubectl delete -f examples/kubernetes/dynamic-provisioning/pvc.yaml
    ```

## More examples and capabilities

### Volume Expansion

Volume expansion can be done by updating and applying corresponding PVC specification.

**Note:** the volume size can only be increased. Any attempts to decrease it will result in an error. It is not recommended to resize Persistent Volume MooseFS-allocated quotas via MooseFS native tools, as such changes will not be visible in your Container Orchestrator.

### Static provisioning

Volumes can be also provisioned statically by creating or using a existing directory in `k8s_root_dir/driver_working_dir/volumes`. Example PersistentVolume `examples/kubernetes/static-provisioning/pv.yaml` definition, requires existing volume in volumes directory.

### Mount MooseFS inside containers

It is possible to mount any MooseFS directory inside containers using static provisioning.

1.  Create a Persistent Volume (`examples/kubernetes/mount-volume/pv.yaml`):

    ```
    kind: PersistentVolume
    apiVersion: v1
    metadata:
      name: my-moosefs-pv-mount
    spec:
      storageClassName: ""               # empty Storage Class
      capacity:
        storage: 1Gi                     # required, however does not have any effect
      accessModes:
        - ReadWriteMany
      csi:
        driver: csi.moosefs.com
        volumeHandle: my-mount-volume   # unique volume name
        volumeAttributes:
          mfsSubDir: "/"                 # subdirectory to be mounted as a rootdir (inside k8s_root_dir)
    ```
   
2.  Create corresponding Persistent Volume Claim (`examples/kubernetes/mount-volume/pvc.yaml`):

    ```
    kind: PersistentVolumeClaim
    apiVersion: v1
    metadata:
      name: my-moosefs-pvc-mount
    spec:
      storageClassName: ""               # empty Storage Class
      volumeName: my-moosefs-pv-mount
      accessModes:
        - ReadWriteMany
      resources:
        requests:
          storage: 1Gi                   # at least as much as in PV, does not have any effect
    ```   
   
3.  Apply both configurations:

    ```
    $ kubectl apply -f examples/kubernetes/mount-volume/pv.yaml
    $ kubectl apply -f examples/kubernetes/mount-volume/pvc.yaml
    ```
   
4.  Verify that PVC exists and wait until it is bound to the previously created PV:

    ```
    $ kubectl get pvc
    NAME                    STATUS   VOLUME                 CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    my-moosefs-pvc-mount    Bound     my-moosefs-pv-mount   1Gi        RWX                           23m
    ```
   
5.  Create a sample workload that mounts the volume:

    ```
    $ kubectl apply -f examples/kubernetes/mount-volume/pod.yaml
    ```
   
6.  Verify that the storage is mounted:

    ```
    $ kubectl exec -it my-moosefs-pod-mount -- ls /data
    ```
   
   You should see the content of `k8s_root_dir/mfsSubDir`.
   
7.  Clean up:

    ```
    $ kubectl delete -f examples/kubernetes/mount-volume/pod.yaml
    $ kubectl delete -f examples/kubernetes/mount-volume/pvc.yaml
    $ kubectl delete -f examples/kubernetes/mount-volume/pv.yaml
    ```

By using `containers[*].volumeMounts[*].subPath` field of `PodSpec` it is possible to specify a proper MooseFS subdirectory using only one PV/PVC pair, without creating a new one for each subdirectory:

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
            claimName: my-moosefs-pvc-mount
```

## Version Compatibility

| Kubernetes | MooseFS CSI Driver |
|:----------:|:------------------:|
| `v1.26.2`  |      `v0.9.4`      |
| `v1.24.2`  |      `v0.9.4`      |

## Copyright

Copyright (c) 2020-2023 Saglabs SA

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at [http://www.apache.org/licenses/LICENSE-2.0](https://www.apache.org/licenses/LICENSE-2.0).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.

See the License for the specific language governing permissions and limitations under the License.

## License

[Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0)

## Code of conduct

Participation in this project is governed by [Kubernetes/CNCF code of conduct](https://github.com/kubernetes/community/blob/master/code-of-conduct.md)
