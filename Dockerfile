FROM ubuntu:18.04

# Install wget, lsb-release and curl
RUN apt-get update && apt-get install -y wget lsb-release curl fuse libfuse2 tree vim gnupg2

# Add key
RUN wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
RUN . /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

# Install MooseFS client
RUN apt-get update && apt-get install -y moosefs-client

CMD ["/bin/bash"]
