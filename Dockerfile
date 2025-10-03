ARG BASE_IMAGE
FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS build

ARG GOOS
ENV GOOS=${GOOS:-linux}

ARG GOARCH
ENV GOARCH=${GOARCH:-amd64}

RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go1.23.10.${GOOS}-${GOARCH}.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.23.10.${GOOS}-${GOARCH}.tar.gz && \
    rm go1.23.10.${GOOS}-${GOARCH}.tar.gz

ENV PATH=$PATH:/usr/local/go/bin

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go mod download

ARG VERSION
ENV VERSION=${VERSION:-1.0.0}

RUN CGO_ENABLED=1 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -ldflags "-X main.Version=${VERSION} -s -w"

FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS exporter
LABEL org.opencontainers.image.authors="Oracle America, Inc."
LABEL org.opencontainers.image.description="Oracle Database Observability Exporter"

ARG VERSION
ENV VERSION=${VERSION:-1.0.0}
ENV DEBIAN_FRONTEND=noninteractive

ARG GOARCH
ENV GOARCH=${GOARCH:-amd64}

RUN microdnf update && \
    microdnf install -y oracle-instantclient-release-23ai-el8-1.0-4.el8 && \
    microdnf install -y oracle-instantclient-basic-23.9.0.25.07-1.el8 && \
    microdnf install -y glibc-2.28-251.0.3.el8_10.25 && \
    microdnf clean all

ENV LD_LIBRARY_PATH=/usr/lib/oracle/23/client64/lib
ENV PATH=$PATH:/usr/lib/oracle/23/client64/bin

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

# create the mount point for alert log exports (default location)
RUN mkdir /log && chown 1000:1000 /log
RUN mkdir /wallet && chown 1000:1000 /wallet

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
