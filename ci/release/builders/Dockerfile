FROM centos:7

ARG GOVERSION=

RUN curl -fsSL "https://go.dev/dl/go$GOVERSION.tar.gz" >/tmp/go.tar.gz
RUN tar -C /usr/local -xzf /tmp/go.tar.gz

ENV PATH="/usr/local/go/bin:$PATH"

RUN yum install -y rsync
RUN yum groupinstall -y 'Development Tools'
