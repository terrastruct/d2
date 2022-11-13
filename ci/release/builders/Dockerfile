FROM centos:7

ARG GOVERSION=1.19.3

RUN curl -fsSL "https://go.dev/dl/go$GOVERSION.linux-amd64.tar.gz" >/tmp/go.tar.gz
RUN tar -C /usr/local -xzf /tmp/go.tar.gz

ENV PATH="/usr/local/go/bin:$PATH"

RUN yum install -y rsync
RUN yum groupinstall -y 'Development Tools'
