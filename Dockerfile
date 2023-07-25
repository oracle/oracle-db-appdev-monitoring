ARG BASE_IMAGE
# Build is starting here
#FROM docker.io/library/golang:1.19 AS build
FROM ${BASE_IMAGE} AS build

RUN microdnf install golang-1.19.10

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go get -d -v

ARG VERSION
ENV VERSION ${VERSION:-0.1.0}

RUN GOOS=linux GOARCH=amd64 go build -v -ldflags "-X main.Version=${VERSION} -s -w"

FROM ${BASE_IMAGE} as exporter
LABEL org.opencontainers.image.authors="Oracle America, Inc."
LABEL org.opencontainers.image.description="Oracle Database Observability Exporter"

ENV VERSION ${VERSION:-0.1.0}
ENV DEBIAN_FRONTEND=noninteractive

RUN  microdnf install -y oracle-instantclient-release-el8 && microdnf install -y oracle-instantclient-basic

ENV LD_LIBRARY_PATH /usr/lib/oracle/21/client64/lib
ENV PATH $PATH:/usr/lib/oracle/21/client64/bin
ENV ORACLE_HOME /usr/lib/oracle/21/client64

#ARG LEGACY_TABLESPACE
#ENV LEGACY_TABLESPACE=${LEGACY_TABLESPACE}
COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
#ADD ./default-metrics${LEGACY_TABLESPACE}.toml /default-metrics.toml

ENV DATA_SOURCE_NAME system/oracle@oracle/xe

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
