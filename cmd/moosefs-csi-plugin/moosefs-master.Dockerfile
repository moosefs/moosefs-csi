# moosefs-master in a single container
# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# Install wget, lsb-release and curl
RUN apt-get update && apt-get install -y wget gnupg2

# Add key
RUN wget -O - http://ppa.moosefs.com/moosefs.key | apt-key add -
RUN . /etc/lsb-release && echo "deb http://ppa.moosefs.com/moosefs-3/apt/ubuntu/$DISTRIB_CODENAME $DISTRIB_CODENAME main" > /etc/apt/sources.list.d/moosefs.list

# Install MooseFS master server
RUN apt-get update && apt-get install -y moosefs-master

# Expose master ports
EXPOSE 9419 9420 9421

# Start master server in the foreground
CMD [ "mfsmaster", "start", "-f" ]
