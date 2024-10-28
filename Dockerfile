ARG BASE_IMAGE
FROM ${BASE_IMAGE} AS build

ARG GOOS
ENV GOOS ${GOOS:-linux}

ARG GOARCH
ENV GOARCH ${GOARCH:-amd64}

RUN microdnf install wget gzip gcc && \
    wget -q https://go.dev/dl/go1.22.7.${GOOS}-${GOARCH}.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.22.7.${GOOS}-${GOARCH}.tar.gz && \
    rm go1.22.7.${GOOS}-${GOARCH}.tar.gz

ENV PATH $PATH:/usr/local/go/bin

WORKDIR /go/src/oracledb_exporter
COPY . .
RUN go get -d -v

ARG VERSION
ENV VERSION ${VERSION:-1.0.0}

RUN CGO_ENABLED=1 GOOS=${GOOS} GOARCH=${GOARCH} go build -v -ldflags "-X main.Version=${VERSION} -s -w"

FROM ${BASE_IMAGE} as exporter
LABEL org.opencontainers.image.authors="Oracle America, Inc."
LABEL org.opencontainers.image.description="Oracle Database Observability Exporter"

ENV VERSION ${VERSION:-1.0.0}
ENV DEBIAN_FRONTEND=noninteractive

ARG GOARCH
ENV GOARCH ${GOARCH:-amd64}

RUN if [ "$GOARCH" = "amd64" ]; then \
      microdnf install -y oracle-instantclient-release-23ai-el8 && microdnf install -y oracle-instantclient-basic && \
      microdnf install glibc-2.28-251.0.2.el8_10.4 \
    ; else \
      wget https://download.oracle.com/otn_software/linux/instantclient/instantclient-basic-linux-arm64.rpm && \
      microdnf localinstall -y instantclient-basic-linux-arm64.rpm && \
      microdnf install glibc-2.28-251.0.2.el8_10.4 \
    ; fi

ENV LD_LIBRARY_PATH /usr/lib/oracle/23/client64/lib
ENV PATH $PATH:/usr/lib/oracle/23/client64/bin
ENV ORACLE_HOME /usr/lib/oracle/23/client64

COPY --from=build /go/src/oracledb_exporter/oracle-db-appdev-monitoring /oracledb_exporter
ADD ./default-metrics.toml /default-metrics.toml

# create the mount point for alert log exports (default location)
RUN mkdir /log && chown 1000:1000 /log

EXPOSE 9161

USER 1000

ENTRYPOINT ["/oracledb_exporter"]
