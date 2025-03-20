ARCH           ?= $(shell uname -m)
OS_TYPE        ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH_TYPE      ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))
GOOS           ?= $(shell go env GOOS)
GOARCH         ?= $(shell go env GOARCH)
VERSION        ?= 1.6.0
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
BUILD_ARGS      = --build-arg VERSION=$(VERSION)
OUTDIR          = ./dist

IMAGE_NAME     ?= container-registry.oracle.com/database/observability-exporter
IMAGE_ID       ?= $(IMAGE_NAME):$(VERSION)
IMAGE_ID_LATEST?= $(IMAGE_NAME):latest

ORACLE_LINUX_BASE_IMAGE ?= ghcr.io/oracle/oraclelinux:8-slim

ifeq ($(GOOS), windows)
EXT?=.exe
else
EXT?=
endif

export LD_LIBRARY_PATH

version:
	@echo "$(VERSION)"

.PHONY: go-build
go-build:
	@echo "Build $(OS_TYPE)"
	mkdir -p $(OUTDIR)/oracledb_exporter-$(VERSION).$(GOOS)-$(GOARCH)/
	go build $(GOFLAGS) -o $(OUTDIR)/oracledb_exporter-$(VERSION).$(GOOS)-$(GOARCH)/oracledb_exporter$(EXT)
	cp default-metrics.toml $(OUTDIR)/$(DIST_DIR)
	#cp teq-default-metrics.toml $(OUTDIR)/$(DIST_DIR)/default-metrics.toml
	(cd dist ; tar cfz oracledb_exporter-$(VERSION).$(GOOS)-$(GOARCH).tar.gz oracledb_exporter-$(VERSION).$(GOOS)-$(GOARCH))

.PHONY: go-build-linux-amd64
go-build-linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 $(MAKE) go-build -j2

.PHONY: go-build-linux-arm64
go-build-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 $(MAKE) go-build -j2

.PHONY: go-build-darwin-amd64
go-build-darwin-amd64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 $(MAKE) go-build -j2

.PHONY: go-build-darwin-arm64
go-build-darwin-arm64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 $(MAKE) go-build -j2

.PHONY: go-build-windows-amd64
go-build-windows-amd64:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 $(MAKE) go-build -j2

.PHONY: go-build-windows-x86
go-build-windows-x86:
	CGO_ENABLED=1 GOOS=windows GOARCH=386 $(MAKE) go-build -j2

go-lint:
	@echo "Linting codebase"
	docker run --rm -v $(shell pwd):/app -v ~/.cache/golangci-lint/v1.50.1:/root/.cache -w /app golangci/golangci-lint:v1.50.1 golangci-lint run -v

local-build: go-build
	@true

build: docker
	@true

deps:
	go get

go-test:
	@echo "Run tests"
	GOOS=$(OS_TYPE) GOARCH=$(ARCH_TYPE) go test -coverprofile="test-coverage.out" $$(go list ./... | grep -v /vendor/)

clean:
	rm -rf ./dist glibc-*.apk oracle-*.rpm

push-images:
	@make --no-print-directory push-oraclelinux-image
	
docker:
	docker build --no-cache --progress=plain $(BUILD_ARGS) -t "$(IMAGE_ID)-amd64" --build-arg BASE_IMAGE=$(ORACLE_LINUX_BASE_IMAGE) --build-arg GOARCH=amd64 . 

docker-arm:
	docker buildx build --platform linux/arm64 --load --no-cache --progress=plain $(BUILD_ARGS) -t "$(IMAGE_ID)-arm64" --build-arg BASE_IMAGE=$(ORACLE_LINUX_BASE_IMAGE) --build-arg GOARCH=arm64 . 

push-oraclelinux-image:
	docker push $(IMAGE_ID)

.PHONY: version build deps go-test clean docker
