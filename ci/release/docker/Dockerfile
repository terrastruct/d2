# https://hub.docker.com/repository/docker/terrastruct/d2
FROM ubuntu:latest

ARG TARGETARCH

RUN apt-get update && apt-get install -y ca-certificates curl dumb-init sudo

RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash -s - && \
  apt-get install -y nodejs
RUN npx playwright@1.52.0 install --with-deps chromium

RUN adduser --gecos '' --disabled-password debian \
  && echo "debian ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/nopasswd

RUN curl -fsSL "https://github.com/boxboat/fixuid/releases/download/v0.5/fixuid-0.5-linux-$TARGETARCH.tar.gz" | tar -C /usr/local/bin -xzf - \
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
RUN d2 init-playwright

WORKDIR /home/debian/src
EXPOSE 8080
ENV PORT 8080
ENV HOST 0.0.0.0
ENV BROWSER false

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
