# moosefs-chunk server in a single container
# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# Install wget, lsb-release and curl
RUN apt-get update && apt-get install -y wget gnupg2

# Add key
RUN wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
RUN . /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

# Install chunk server
RUN apt-get update && apt-get install -y moosefs-chunkserver

# Config: used only while `docker build` the real mounts happens from k8s configMap
RUN mkdir -p /mnt/sdb1 && chown -R mfs:mfs /mnt/sdb1 && echo "/mnt/sdb1 1GiB" >> /etc/mfs/mfshdd.cfg && sed -i '/# LABELS =/c\LABELS = DOCKER' /etc/mfs/mfschunkserver.cfg

# Expose ports
EXPOSE 9422

# Start chunkserver in the foreground
CMD [ "mfschunkserver", "start", "-f" ]
