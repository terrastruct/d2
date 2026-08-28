# https://hub.docker.com/r/d2lang/d2
FROM ubuntu:24.04@sha256:561618e2c15bf2397621dd04f96926663a3b5616c189cf7e38db7e82f5c538ea

ARG TARGETARCH
ARG FIXUID_VERSION=0.6.0

RUN apt-get update && apt-get install -y ca-certificates curl dumb-init sudo

RUN curl -fsSL https://deb.nodesource.com/setup_24.x | bash -s - && \
  apt-get install -y nodejs
RUN npx playwright@1.61.1 install --with-deps chromium

RUN adduser --gecos '' --disabled-password debian \
  && echo "debian ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/nopasswd

RUN case "$TARGETARCH" in \
    amd64) fixuid_sha256=8c47f64ec4eec60e79871796ea4097ead919f7fcdedace766da9510b78c5fa14 ;; \
    arm64) fixuid_sha256=827e0b480c38470b5defb84343be7bb4e85b9efcbf3780ac779374e8b040a969 ;; \
    *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
  esac \
  && curl -fsSL "https://github.com/boxboat/fixuid/releases/download/v$FIXUID_VERSION/fixuid-$FIXUID_VERSION-linux-$TARGETARCH.tar.gz" -o /tmp/fixuid.tar.gz \
  && echo "$fixuid_sha256  /tmp/fixuid.tar.gz" | sha256sum -c - \
  && tar -C /usr/local/bin -xzf /tmp/fixuid.tar.gz \
  && rm /tmp/fixuid.tar.gz \
  && chown root:root /usr/local/bin/fixuid \
  && chmod 4755 /usr/local/bin/fixuid \
  && mkdir -p /etc/fixuid \
  && printf "user: debian\ngroup: debian\npaths: [/home/debian]\n" > /etc/fixuid/config.yml

COPY ./d2-*-linux-$TARGETARCH.tar.gz /tmp
ADD ./entrypoint.sh /usr/local/bin/entrypoint.sh
RUN mkdir -p /usr/local/lib/d2 \
      && tar -C /usr/local/lib/d2 -xzf /tmp/d2-*-linux-"$TARGETARCH".tar.gz \
      && /usr/local/lib/d2/d2-*/scripts/install.sh \
      && rm -Rf /tmp/d2-*-linux-"$TARGETARCH".tar.gz

USER debian:debian
RUN CI=1 d2 init-playwright

WORKDIR /home/debian/src
EXPOSE 8080
ENV PORT 8080
ENV HOST 0.0.0.0
ENV BROWSER false

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
