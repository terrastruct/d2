FROM debian:10

RUN apt-get update
RUN apt-get install -y curl

ARG GOVERSION=
RUN curl -fsSL "https://go.dev/dl/go$GOVERSION.tar.gz" >/tmp/go.tar.gz
RUN tar -C /usr/local -xzf /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:$PATH"

RUN apt-get install -y build-essential
RUN apt-get install -y rsync
