.PHONY: build build-xftp clean install uninstall run run-xftp tui list test race fmt vet deps tidy check dev

BINARY_NAME=xssh
BINARY_XFTP=xftp
BUILD_DIR=./build
INSTALL_DIR=/usr/local/bin

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-ldflags "-X github.com/ketor/xsc/pkg/version.Version=$(VERSION) -X github.com/ketor/xsc/pkg/version.GitCommit=$(GIT_COMMIT) -X github.com/ketor/xsc/pkg/version.BuildDate=$(BUILD_DATE)"

# 构建 xssh 和 xftp
build:
	mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/xssh
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_XFTP) ./cmd/xftp
# 构建 xftp
build-xftp:
	mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_XFTP) ./cmd/xftp



# 清理构建文件
clean:
	rm -rf $(BUILD_DIR)

# 安装到系统（自动 sudo）
install: build
	sudo install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	sudo install -m 755 $(BUILD_DIR)/$(BINARY_XFTP) $(INSTALL_DIR)/

# 卸载
uninstall:
	sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	sudo rm -f $(INSTALL_DIR)/$(BINARY_XFTP)

# 运行 xssh
run:
	go run ./cmd/xssh

# 运行 xftp
run-xftp:
	go run ./cmd/xftp

# 带参数运行
tui:
	go run ./cmd/xssh

list:
	go run ./cmd/xssh list

# 测试
test:
	go test -v ./...

# 格式化代码
fmt:
	go fmt ./...

# 检查代码
vet:
	go vet ./...
# 下载依赖（不修改 go.mod/go.sum）
deps:
	go mod download

tidy:
	go mod tidy

race:
	go test -race ./...

check: fmt vet test race

dev:
	@command -v air >/dev/null || { echo "air 未安装；请使用固定版本手动安装"; exit 1; }
	air
