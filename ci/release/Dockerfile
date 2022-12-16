# https://hub.docker.com/repository/docker/terrastruct/d2
FROM debian:latest

ARG TARGETARCH

COPY ./d2-*-linux-$TARGETARCH.tar.gz /tmp
RUN mkdir -p /usr/local/lib/d2 \
      && tar -C /usr/local/lib/d2 -xzf /tmp/d2-*-linux-"$TARGETARCH".tar.gz \
      && /usr/local/lib/d2/d2-*/scripts/install.sh \
      && rm -Rf /tmp/d2-*-linux-"$TARGETARCH".tar.gz

WORKDIR /root/src
EXPOSE 8080
ENV PORT 8080
ENV HOST 0.0.0.0
ENV BROWSER false

ENTRYPOINT ["/usr/local/bin/d2"]
