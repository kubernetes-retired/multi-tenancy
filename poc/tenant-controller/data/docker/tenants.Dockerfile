FROM debian:stretch
RUN apt-get -y update && apt-get -y install ca-certificates && apt-get -y clean
ADD tenant-ctl /bin/
ENTRYPOINT ["/bin/tenant-ctl"]
