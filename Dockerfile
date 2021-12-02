################################################################################
# daemon
################################################################################
FROM golang:1.13.5-stretch AS build-env

WORKDIR /app/

ARG GET_CMD
ARG ARCH
ARG DPKG_DOMAIN

ENV ARCH=amd64 \
    GO111MODULE=on

RUN dpkg --add-architecture ${ARCH} && \
    apt update

COPY ./ ./
COPY ./.ci/s3curl /usr/local/bin/s3curl
RUN GET_CMD=s3curl  DPKG_DOMAIN=https://mxswdc2.s3-ap-northeast-1.amazonaws.com make dpkgs-dev
RUN make dpkgs-runtime
RUN CLOUD=monitor make remoted

################################################################################
# target
################################################################################

FROM debian:9-slim

RUN apt-get update && apt-get install wget procps curl jq -y 

# prometheus >>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>
RUN useradd -M -r -s /bin/false prometheus

RUN mkdir /etc/prometheus
RUN mkdir /var/lib/prometheus
RUN chown prometheus:prometheus /etc/prometheus
RUN chown prometheus:prometheus /var/lib/prometheus

RUN wget https://github.com/prometheus/prometheus/releases/download/v2.6.0/prometheus-2.6.0.linux-amd64.tar.gz
RUN tar -xzf prometheus-2.6.0.linux-amd64.tar.gz

RUN cp prometheus-2.6.0.linux-amd64/prometheus /usr/local/bin/
RUN cp prometheus-2.6.0.linux-amd64/promtool /usr/local/bin/

RUN chown prometheus:prometheus /usr/local/bin/prometheus
RUN chown prometheus:prometheus /usr/local/bin/promtool

RUN cp -r prometheus-2.6.0.linux-amd64/consoles /etc/prometheus/
RUN cp -r prometheus-2.6.0.linux-amd64/console_libraries/ /etc/prometheus/

RUN chown -R prometheus:prometheus /etc/prometheus/consoles
RUN chown -R prometheus:prometheus /etc/prometheus/console_libraries
COPY cmd/monitor/prometheus/prometheus.yml /etc/prometheus/
COPY --from=build-env /app/build/amd64/monitor/remoted /usr/sbin/
# prometheus <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<


COPY cmd/monitor/entrypoint.sh /usr/sbin/
CMD "/usr/sbin/entrypoint.sh"
