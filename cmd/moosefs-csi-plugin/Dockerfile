FROM alpine:3.7

RUN apk add --no-cache ca-certificates e2fsprogs findmnt

ADD moosefs-csi-plugin /bin/

ENTRYPOINT ["/bin/moosefs-csi-plugin"]
