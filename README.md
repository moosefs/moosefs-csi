# Container Storage Interface (CSI) for MooseFs
[Container storage interface](https://kubernetes-csi.github.io/docs/) is an [industry standard](https://github.com/container-storage-interface/spec/blob/master/spec.md) that will enable storage vendors to develop a plugin once and have it work across a number of container orchestration systems.

MooseFs is an open-source distributed file system which aims to be fault-tolerant, highly available, highly performing, scalable general-purpose network distributed file system for data centers.

## Introduction
Similar to other storage providers MooseFs can act as a layer on top of hybrid storage. The storage can be distributed across multiple private/public clouds.

[//]: # "image courtesy: "
![alt MooseFSCSI](MooseFSinCSI.png)


## Deployment (Kubernetes)
#### Prerequisites:
* Already have a working Kubernetes cluster
* AWS/GCP/Azure credentials available

### AWS EBS storage for Google Kubernetes Engine (GKE)
1. Apply the container storage interface for moosefs for your cluster
```
$ kubectl apply -f deploy/kubernetes/moosefs-csi.yaml
```
2. Ensure all the containers are ready and running
```
$ kubectl get po -n kube-system
```
3. Testing: Create a persistant volume claim for 5GiB with name `moosefs-csi-pvc` with storage class `moosefs-block-storage`
```
$ kubectl apply -f deploy/kubernetes/sample-moosefs-pvc.yaml
```
4. Verify if the persistant volume claim and wait until its the STATUS is `Bound`
```
$ kubectl get pvc
```
5. After its in bound state, create a sample workload mouting that volume
```
$ kubectl apply -f deploy/kubernetes/sample-busybox-pod.yaml
```
6. Verify the storage mount of the busybox pod
```
$ kubectl exec -it my-csi-app df -h
```

## How it works (Kubernetes)
MooseFs abstracts heterogenous storage providers and acts as a single interface. Here, you can see a kubernetes cluster with moosefs-csi in GKE having storage from AWS elastic block store and Google's persistant disk.
![alt k8sMooseFs](k8sMooseFs.png)


## Debuging
| Section                  | Issues                            |Commands  |
| -------------            |:-------------                     |:-----    |
| Persistance volume claim |pvc is in Pending state            | `kubectl get events` |
|                          || `kubectl logs csi-attacher-moosefs-plugin-0 -c moosefs-csi-plugin -n kube-system` |
|                          || `kubectl logs csi-provisioner-moosefs-plugin-0 -c moosefs-csi-plugin -n kube-system` |

## Miscelleneous
| Description                        | Command       |
| -------------                      |:------------- |
|AWS session token creation          |`aws sts get-session-token --duration-seconds 129600` |
|Docker command for launching moosefs|`docker run --cap-add SYS_ADMIN --security-opt apparmor:unconfined -v /dev/fuse:/dev/fuse --privileged -it mfs /bin/bash`



## TODO
* Possibility to define the count of physical disks on servers
* Possibility to define the count of chunk servers
* Automatically generate the best mooseFs configration based on user's need/metrics (how many chunks, topology of servers etc.)
* Possibility to define replication goal (Erosure codes)

#### Minio, object storage
Tested with docker containers on a local vm, not so good perf.
```
/ # time mc cp archive.tgz play/test1
archive.tgz:    363.41 MB / 363.41 MB ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 100.00% 1.04 MB/s 5m49sreal	5m 52.39s
user	0m 1.40s
sys	0m 1.49s
/ #
```
