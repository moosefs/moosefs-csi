# moosefs-client in a single container
# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# Install wget, lsb-release and curl
RUN apt update && apt install -y wget gnupg2 fuse libfuse2 ca-certificates e2fsprogs

# Add key
RUN wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
RUN . /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

# Install MooseFS master server
RUN apt update && apt install -y moosefs-client

# Start master server in the foreground
CMD [ "/bin/bash" ]
