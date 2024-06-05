ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS build

RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go1.22.4.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz && \
    rm go1.22.4.linux-amd64.tar.gz

ENV PATH $PATH:/usr/local/go/bin

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go get -d -v

ARG VERSION
ENV VERSION ${VERSION:-1.0.0}

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -ldflags "-X main.Version=${VERSION} -s -w"

FROM ${BASE_IMAGE} as exporter
LABEL org.opencontainers.image.authors="Oracle America, Inc."
LABEL org.opencontainers.image.description="Oracle Database Observability Exporter"

ENV VERSION ${VERSION:-1.0.0}
ENV DEBIAN_FRONTEND=noninteractive

RUN microdnf install -y oracle-instantclient-release-el8 && microdnf install -y oracle-instantclient-basic

ENV LD_LIBRARY_PATH /usr/lib/oracle/21/client64/lib
ENV PATH $PATH:/usr/lib/oracle/21/client64/bin
ENV ORACLE_HOME /usr/lib/oracle/21/client64

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
