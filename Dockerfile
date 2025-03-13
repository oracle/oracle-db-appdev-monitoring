ARG BASE_IMAGE
FROM ${BASE_IMAGE:-ghcr.io/oracle/oraclelinux:8-slim} AS build

ARG GOOS
ENV GOOS=${GOOS:-linux}

ARG GOARCH
ENV GOARCH=${GOARCH:-amd64}

RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go1.23.7.${GOOS}-${GOARCH}.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.23.7.${GOOS}-${GOARCH}.tar.gz && \
    rm go1.23.7.${GOOS}-${GOARCH}.tar.gz

ENV PATH=$PATH:/usr/local/go/bin

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go get -d -v

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

# note: the 23ai arm drivers are not in yum yet, when they are can switch this to just use yum and not need
# to wget the driver for arm.  also note that the permalink for otn download of drivers does not have version
# in it, and does not appear to be a link vith version in it, so that link is very brittle and could break build

# second note: moved back to 21c drivers due to adb-s non-root connection issue. for 23ai, change rpm to
# oracle-instantclient-release-23ai-el8 and paths below s/21/23/
RUN if [ "$GOARCH" = "amd64" ]; then \
      microdnf install -y oracle-instantclient-release-el8 && microdnf install -y oracle-instantclient-basic && \
      microdnf install glibc-2.28-251.0.2.el8_10.4 \
    ; else \
      microdnf install wget libaio && \
      wget https://download.oracle.com/otn_software/linux/instantclient/instantclient-basic-linux-arm64.rpm && \
      rpm -ivh instantclient-basic-linux-arm64.rpm && \
      ln -s /usr/lib/oracle/19.24 /usr/lib/oracle/21 && \
      microdnf install glibc-2.28-251.0.2.el8_10.4 \
    ; fi

ENV LD_LIBRARY_PATH=/usr/lib/oracle/21/client64/lib:usr/lib/oracle/19.24/client64/lib
ENV PATH=$PATH:/usr/lib/oracle/21/client64/bin:usr/lib/oracle/19.24/client64/bin

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

# create the mount point for alert log exports (default location)
RUN mkdir /log && chown 1000:1000 /log
RUN mkdir /wallet && chown 1000:1000 /wallet

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
