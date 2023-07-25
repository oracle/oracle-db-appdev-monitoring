ARCH           ?= $(shell uname -m)
OS_TYPE        ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH_TYPE      ?= $(subst x86_64,amd64,$(patsubst i%86,386,$(ARCH)))
GOOS           ?= $(shell go env GOOS)
GOARCH         ?= $(shell go env GOARCH)
VERSION        ?= 0.99.6
LDFLAGS        := -X main.Version=$(VERSION)
GOFLAGS        := -ldflags "$(LDFLAGS) -s -w"
BUILD_ARGS      = --build-arg VERSION=$(VERSION)
LEGACY_TABLESPACE = --build-arg LEGACY_TABLESPACE=.legacy-tablespace
OUTDIR          = ./dist

IMAGE_NAME     ?= oracle/observability-exporter
IMAGE_ID       ?= $(IMAGE_NAME):$(VERSION)
IMAGE_ID_LATEST?= $(IMAGE_NAME):latest
RELEASE        ?= true

#UBUNTU_BASE_IMAGE       ?= docker.io/library/ubuntu:23.04
ORACLE_LINUX_BASE_IMAGE ?= ghcr.io/oracle/oraclelinux:8-slim
#ALPINE_BASE_IMAGE       ?= docker.io/library/alpine:3.17

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
	#cp default-metrics.toml $(OUTDIR)/$(DIST_DIR)
	cp teq-default-metrics.toml $(OUTDIR)/$(DIST_DIR)/default-metrics.toml
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
	rm -rf ./dist sgerrand.rsa.pub glibc-*.apk oracle-*.rpm

#docker: ubuntu-image alpine-image oraclelinux-image
docker: oraclelinux-image

push-images:
	@make --no-print-directory push-ubuntu-image
	@make --no-print-directory push-oraclelinux-image
	@make --no-print-directory push-alpine-image

oraclelinux-image:
	if DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect "$(IMAGE_ID)-oraclelinux" > /dev/null; then \
		echo "Image \"$(IMAGE_ID)-oraclelinux\" already exists on ghcr.io"; \
	else \
		docker build --progress=plain $(BUILD_ARGS) -t "$(IMAGE_ID)-oraclelinux" --build-arg BASE_IMAGE=$(ORACLE_LINUX_BASE_IMAGE) . && \
		#docker build --progress=plain $(BUILD_ARGS) $(LEGACY_TABLESPACE) -t "$(IMAGE_ID)-oraclelinux_legacy-tablespace" --build-arg BASE_IMAGE=$(ORACLE_LINUX_BASE_IMAGE) . && \
		docker tag "$(IMAGE_ID)-oraclelinux" "$(IMAGE_NAME):oraclelinux"; \
	fi

push-oraclelinux-image:
	docker push $(IMAGE_ID)-oraclelinux
ifeq ("$(RELEASE)", "true")
	docker push "$(IMAGE_NAME):oraclelinux"
	docker push "$(IMAGE_ID)-oraclelinux_legacy-tablespace"
endif

sign-oraclelinux-image:
ifneq ("$(wildcard cosign.key)","")
	cosign sign --key cosign.key $(IMAGE_ID)-oraclelinux
else
	@echo "Can't find cosign.key file"
endif

ubuntu-image:
	if DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect "$(IMAGE_ID)" > /dev/null; then \
		echo "Image \"$(IMAGE_ID)\" already exists on ghcr.io"; \
	else \
		docker build --progress=plain $(BUILD_ARGS) --build-arg BASE_IMAGE=$(UBUNTU_BASE_IMAGE) -t "$(IMAGE_ID)" . && \
		docker build --progress=plain $(BUILD_ARGS) --build-arg BASE_IMAGE=$(UBUNTU_BASE_IMAGE) $(LEGACY_TABLESPACE) -t "$(IMAGE_ID)_legacy-tablespace" . && \
		docker tag "$(IMAGE_ID)" "$(IMAGE_ID_LATEST)"; \
	fi

push-ubuntu-image:
	docker push $(IMAGE_ID)
ifeq ("$(RELEASE)", "true")
	docker push "$(IMAGE_ID_LATEST)"
	docker push "$(IMAGE_ID)_legacy-tablespace"
endif

sign-ubuntu-image:
ifneq ("$(wildcard cosign.key)","")
	cosign sign --key cosign.key $(IMAGE_ID)
	cosign sign --key cosign.key $(IMAGE_ID_LATEST)
else
	@echo "Can't find cosign.key file"
endif

alpine-image:
	if DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect "$(IMAGE_ID)-alpine" > /dev/null; then \
		echo "Image \"$(IMAGE_ID)-alpine\" already exists on ghcr.io"; \
	else \
		docker build --progress=plain $(BUILD_ARGS) -t "$(IMAGE_ID)-alpine" --build-arg BASE_IMAGE=$(ALPINE_BASE_IMAGE) . && \
		docker build --progress=plain $(BUILD_ARGS) $(LEGACY_TABLESPACE) --build-arg BASE_IMAGE=$(ALPINE_BASE_IMAGE) -t "$(IMAGE_ID)-alpine_legacy-tablespace" . && \
		docker tag "$(IMAGE_ID)-alpine" "$(IMAGE_NAME):alpine"; \
	fi

push-alpine-image:
	docker push $(IMAGE_ID)-alpine
ifeq ("$(RELEASE)", "true")
	docker push "$(IMAGE_NAME):alpine"
endif

sign-alpine-image:
ifneq ("$(wildcard cosign.key)","")
	cosign sign --key cosign.key $(IMAGE_ID)-alpine
else
	@echo "Can't find cosign.key file"
endif

.PHONY: version build deps go-test clean docker
