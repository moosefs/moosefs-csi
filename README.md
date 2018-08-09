## Container Storage Interface (CSI) for mooseFs

#### Docker command
```
docker run --cap-add SYS_ADMIN --security-opt apparmor:unconfined -v /dev/fuse:/dev/fuse --privileged -it mfs /bin/bash
```
