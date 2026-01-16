ARG BASE_IMAGE
FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS build

ARG GOOS
ENV GOOS=${GOOS:-linux}

ARG GOARCH
ENV GOARCH=${GOARCH:-amd64}

ARG TAGS
ENV TAGS=${TAGS:-godror}

ARG CGO_ENABLED
ENV CGO_ENABLED=${CGO_ENABLED:-1}

ARG GO_VERSION=1.24.11
ENV GO_VERSION=${GO_VERSION}

RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz && \
    rm go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz

ENV PATH=$PATH:/usr/local/go/bin

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go mod download

ARG VERSION
ENV VERSION=${VERSION:-1.0.0}

RUN CGO_ENABLED=${CGO_ENABLED} GOOS=${GOOS} GOARCH=${GOARCH} go build --tags=${TAGS} -v -ldflags "-X main.Version=${VERSION} -s -w"

FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS exporter-godror
LABEL org.opencontainers.image.authors="Oracle America, Inc."
LABEL org.opencontainers.image.description="Oracle AI Database Observability Exporter"

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

FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS exporter-goora

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

# create the mount point for alert log exports (default location)
RUN mkdir /log && chown 1000:1000 /log
RUN mkdir /wallet && chown 1000:1000 /wallet

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
