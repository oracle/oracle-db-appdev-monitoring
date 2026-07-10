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

ARG GO_VERSION=1.26.5
ENV GO_VERSION=${GO_VERSION}

RUN microdnf update -y && \
    microdnf install -y wget gzip gcc jq && \
    microdnf clean all && \
    go_tarball="go${GO_VERSION}.${GOOS}-${GOARCH}.tar.gz" && \
    wget -q "https://go.dev/dl/${go_tarball}" && \
    go_checksum="$(wget -qO- 'https://go.dev/dl/?mode=json' | jq -r --arg version "go${GO_VERSION}" --arg os "${GOOS}" --arg arch "${GOARCH}" '.[] | select(.version == $version) | .files[] | select(.os == $os and .arch == $arch) | .sha256' | head -n 1)" && \
    test -n "${go_checksum}" && test "${go_checksum}" != "null" && \
    printf '%s  %s\n' "${go_checksum}" "${go_tarball}" | sha256sum -c - && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf "${go_tarball}" && \
    rm "${go_tarball}"

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

RUN microdnf update -y && \
    microdnf install --setopt=install_weak_deps=0 -y oracle-instantclient-release-23ai-el8 && \
    microdnf install -y oracle-instantclient-basic glibc && \
    microdnf update -y expat gnutls glibc && \
    rpm -e --nodeps platform-python-setuptools && \
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

RUN rpm -e --nodeps platform-python-setuptools && \
    microdnf clean all

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

# create the mount point for alert log exports (default location)
RUN mkdir /log && chown 1000:1000 /log
RUN mkdir /wallet && chown 1000:1000 /wallet

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
