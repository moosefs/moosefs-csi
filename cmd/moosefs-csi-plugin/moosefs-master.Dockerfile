# moosefs-master in a single container
# TODO(anoop): Should be moved upstream
FROM ubuntu:18.04

# As much lesser layers as possible.
RUN apt-get update && \
    apt-get install -y wget && \
    wget http://ppa.moosefs.com/moosefs-3/apt/ubuntu/bionic/pool/main/m/moosefs/moosefs-master_3.0.101-1_amd64.deb && \
    dpkg -i moosefs-master_3.0.101-1_amd64.deb

# Expose master ports
EXPOSE 9419 9420 9421

# Start master server in the foreground
CMD [ "mfsmaster", "start", "-f" ]
