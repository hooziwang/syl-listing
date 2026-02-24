APP := syl-listing
GO ?= go
BIN_DIR ?= bin
BIN := $(BIN_DIR)/$(APP)
DESTDIR ?=
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X 'syl-listing/cmd.Version=$(VERSION)' -X 'syl-listing/cmd.Commit=$(COMMIT)' -X 'syl-listing/cmd.BuildTime=$(BUILD_TIME)'
GO_BIN_DIR ?= $(shell sh -c 'gobin="$$( $(GO) env GOBIN )"; if [ -n "$$gobin" ]; then printf "%s" "$$gobin"; else gopath="$$( $(GO) env GOPATH )"; printf "%s/bin" "$${gopath%%:*}"; fi')
INSTALL_BIN_DIR := $(DESTDIR)$(GO_BIN_DIR)
INSTALL_BIN := $(INSTALL_BIN_DIR)/$(APP)
DEFAULT_GOAL := default

INPUTS ?=
OUT ?=
CONFIG ?=
NUM ?=
CONCURRENCY ?=
MAX_RETRIES ?=
PROVIDER ?=
LOG_FILE ?=

.DEFAULT_GOAL := $(DEFAULT_GOAL)

.PHONY: default help build test fmt tidy clean run run-gen install uninstall

default:
	@$(MAKE) fmt
	@$(MAKE) test
	@$(MAKE) install

help:
	@echo "Targets:"
	@echo "  make              - 默认流程：fmt -> test -> install"
	@echo "  make build        - 编译二进制到 $(BIN)"
	@echo "  make test         - 运行全部测试"
	@echo "  make fmt          - gofmt 全部 Go 文件"
	@echo "  make tidy         - 整理 go.mod/go.sum"
	@echo "  make run          - 直跑入口（需要 INPUTS）"
	@echo "  make run-gen      - gen 子命令入口（需要 INPUTS）"
	@echo "  make install      - 安装到 Go bin 目录（GOBIN 或 GOPATH/bin）"
	@echo "  make uninstall    - 卸载已安装二进制"
	@echo "  make clean        - 删除构建产物"
	@echo ""
	@echo "Variables:"
	@echo "  INPUTS='a.md b.md /path/dir'  必填（run/run-gen）"
	@echo "  OUT=/path/outdir              可选"
	@echo "  CONFIG=/path/config.yaml      可选"
	@echo "  NUM=3                         可选"
	@echo "  CONCURRENCY=10               可选"
	@echo "  MAX_RETRIES=3                可选"
	@echo "  PROVIDER=openai              可选"
	@echo "  LOG_FILE=/path/log.ndjson    可选"
	@echo "  GO_BIN_DIR=...               覆盖安装目录（默认 GOBIN 或 GOPATH/bin）"
	@echo "  DESTDIR=                     打包场景根目录"
	@echo "  VERSION=v0.1.0               可选，覆盖版本号"
	@echo "  COMMIT=abc1234               可选，覆盖提交哈希"
	@echo "  BUILD_TIME=...               可选，覆盖构建时间（UTC）"

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) .

test:
	$(GO) test ./...

fmt:
	@gofmt -w $$(find . -name '*.go' -type f)

tidy:
	$(GO) mod tidy

run:
	@if [ -z "$(INPUTS)" ]; then echo "还没传 INPUTS（文件/目录）"; exit 1; fi
	$(GO) run . $(INPUTS) $(if $(OUT),-o "$(OUT)",) $(if $(CONFIG),--config "$(CONFIG)",) $(if $(NUM),-n $(NUM),) $(if $(CONCURRENCY),--concurrency $(CONCURRENCY),) $(if $(MAX_RETRIES),--max-retries $(MAX_RETRIES),) $(if $(PROVIDER),--provider "$(PROVIDER)",) $(if $(LOG_FILE),--log-file "$(LOG_FILE)",)

run-gen:
	@if [ -z "$(INPUTS)" ]; then echo "还没传 INPUTS（文件/目录）"; exit 1; fi
	$(GO) run . gen $(INPUTS) $(if $(OUT),-o "$(OUT)",) $(if $(CONFIG),--config "$(CONFIG)",) $(if $(NUM),-n $(NUM),) $(if $(CONCURRENCY),--concurrency $(CONCURRENCY),) $(if $(MAX_RETRIES),--max-retries $(MAX_RETRIES),) $(if $(PROVIDER),--provider "$(PROVIDER)",) $(if $(LOG_FILE),--log-file "$(LOG_FILE)",)

clean:
	rm -rf $(BIN_DIR)

install: build
	@mkdir -p "$(INSTALL_BIN_DIR)"
	install -m 0755 "$(BIN)" "$(INSTALL_BIN)"

uninstall:
	rm -f "$(INSTALL_BIN)"
